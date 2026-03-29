# Dependencies: Leverage vs. Write From Scratch

## The Central Dependency Decision

kjell has two major components:
1. **Shell parser** — turns command strings into ASTs
2. **Command database + classifier** — walks the AST and classifies

The database + classifier is our core logic — no existing library does this, so we write it. The shell parser is the question.

## Option 1: Depend on mvdan/sh (via shfmt)

This is what safecmd does. Call `shfmt --tojson` as an external process, parse the JSON AST.

**Pros:**
- Battle-tested parser (powers the standard shell formatter)
- Handles bash, POSIX sh, mksh — grammar is complete and well-maintained
- JSON output is stable and well-documented
- Zero parser code to maintain on our end
- We can focus entirely on the classification logic

**Cons:**
- External process dependency — `shfmt` must be installed
- Process spawning overhead per classification (~5-20ms including fork+exec)
- Not embeddable in WASM or browser contexts
- Tight coupling to shfmt's JSON format (if it changes, we break)
- Distributing a binary dependency is friction for library consumers

**Verdict:** Good for a CLI tool or Python/Go library. Bad for an embeddable library. Could be the initial approach.

## Option 2: Depend on mvdan/sh as a Go Library

If we write in Go, we can use `mvdan.cc/sh/v3/syntax` directly.

**Pros:**
- No external process, no serialization overhead
- Full access to the AST with rich type information
- Same battle-tested parser, just used as a library
- Efficient — parse and classify in a single pass

**Cons:**
- Locks the reference implementation to Go
- CGo makes FFI to other languages painful
- Go is not a natural WASM target (large binary, GC overhead)

**Verdict:** Best option if Go is the reference implementation language. The quality of `mvdan/sh` is a strong argument for choosing Go.

## Option 3: Depend on tree-sitter-bash

tree-sitter grammars are C libraries with bindings everywhere (Rust, Python, JS, Go).

**Pros:**
- Native bindings in every language we care about
- Incremental parsing, error recovery built in
- Active community maintaining the bash grammar
- Natural fit for a Rust core (tree-sitter itself is Rust/C)

**Cons:**
- Grammar may not capture all bash nuances that mvdan/sh handles
- tree-sitter is designed for editors (syntax highlighting, code nav) — may not expose semantic details we need
- Dependency on C library complicates some build environments

**Verdict:** Good for a Rust core. Less proven for semantic analysis than mvdan/sh.

## Option 4: Hand-Write a Minimal Parser

Write just enough parser to handle the patterns agents actually generate. Not a full bash parser — a command classifier.

What we actually need to parse:
```
command [flags] [args]           # 80% of cases
cmd1 | cmd2 | cmd3               # pipes
cmd1 && cmd2 ; cmd3              # sequences
cmd > file, cmd >> file          # redirects
$(cmd), `cmd`                    # command substitution
(cmd)                            # subshells
```

What we do NOT need:
- Full arithmetic expansion
- Here-documents (treat as opaque)
- Arrays, associative arrays
- Advanced parameter expansion (`${var//pattern/replacement}`)
- Process substitution (`<(cmd)`) — treat as opaque
- Function definitions

**Pros:**
- No dependencies at all
- Tailored exactly to our needs
- Small, auditable codebase
- Compiles to WASM trivially
- Can be ported to any language

**Cons:**
- We own all the parser bugs
- Shell parsing is notoriously tricky even for a subset
- Quoting rules alone are complex (`"$var"`, `'literal'`, `$'escape'`, backslash)
- Risk of getting it subtly wrong in ways that create security holes

**Verdict:** Tempting for simplicity but risky. Parser bugs in a security-adjacent tool are bad. However, if we scope it tightly and test extensively, it's viable.

## Option 5: Hybrid Approach

Use a battle-tested parser (mvdan/sh or tree-sitter) for the reference implementation and test suite generation, but also provide a minimal parser for embedded use.

```
Reference implementation (Go + mvdan/sh)
  → generates compliance test cases
  → validates correctness

Embedded library (Rust, minimal parser)
  → must pass all compliance tests
  → optimized for embedding
  → no external dependencies
```

**Pros:**
- Best of both worlds: correctness from a proven parser, embeddability from a minimal one
- Compliance tests catch any divergence between parsers
- Consumers choose which they want: full-featured or minimal

**Cons:**
- Two parsers to maintain
- More complex project structure

**Verdict:** This might be the right long-term architecture, but it's overengineered for Phase 1.

## Decision

**Go + mvdan/sh as a library (Option 2).** Reasons:

1. mvdan/sh is the best shell parser available — classifications are based on correct parses
2. safecmd already validates the shfmt-based approach
3. We focus 100% on the database and classification logic instead of fighting parser edge cases
4. The compliance test suite is the contract for any future language ports

## Other Dependencies

### Serialization (database format)
- **TOML**: human-readable, good for the canonical database format
- **JSON**: machine-friendly export format
- Dependency: one TOML parser per language (small, stable, ubiquitous)

### Testing
- Standard test frameworks per language — no exotic dependencies
- Compliance test files are language-independent data (TOML/JSON)

### CLI
- Argument parsing: standard library or lightweight dep per language
- No framework needed — this is a simple command-line tool

### General Principle
Keep the dependency tree shallow. One parser library + one serialization library + standard library. No frameworks, no build system plugins, no codegen tools.
