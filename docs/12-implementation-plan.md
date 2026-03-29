# Implementation Plan

Test-driven, layer by layer. Each layer has a clean interface, is independently testable, and doesn't know about the layers above it.

## Architecture

```
┌─────────────────────────────────────────────┐
│  CLI / Adapters                             │
│  (kjell check, --format claude-code, etc.)  │
├─────────────────────────────────────────────┤
│  Classifier                                 │
│  (walks parsed commands, applies DB rules)  │
├───────────────────┬─────────────────────────┤
│  Parser           │  Database               │
│  (shell string    │  (loads & queries        │
│   → command AST)  │   command definitions)   │
└───────────────────┴─────────────────────────┘
```

Four layers, bottom-up:

1. **Database** — loads TOML definitions, provides lookups
2. **Parser** — turns shell strings into a command-oriented AST
3. **Classifier** — walks parsed commands against the database, handles recursion
4. **CLI / Adapters** — user-facing: CLI flags, format adapters (claude-code, json, plain)

---

## Layer Interfaces

### Database

```
// Types
CommandDef {
  name: string
  default: "read" | "write" | "unknown"
  flags: FlagDef[]
  subcommands: map[string]CommandDef
  recursive: bool
  inner_command_position: int | "after_vars" | null
  separator: string | null
}

FlagDef {
  flag: string[]              // ["-i", "--in-place"]
  effect: "read" | "write" | "unknown" | "recursive"
  reason: string
  inner_command_terminators: string[] | null   // for recursive flags
  inner_command_source: "next_arg_as_shell" | "trailing_args_as_shell" | null
  values: map[string]string | null            // for value-dependent flags like -X GET/POST
}

// Interface
Database {
  lookup(command: string) -> CommandDef | null
}
```

### Parser

```
// Types — command-oriented, not a full shell AST
// We don't need the full tree-sitter/mvdan AST exposed.
// We need: what commands are being run, with what flags/args, how are they composed.

ParsedCommand {
  command: string             // "grep", "git"
  args: string[]              // everything after the command name
  flags: ParsedFlag[]         // parsed out of args
  subcommand: string | null   // "log" for "git log"
}

ParsedFlag {
  name: string                // "-i", "--in-place"
  value: string | null        // "POST" for "-X POST"
}

ParsedPipeline {
  commands: ParsedCommand[]   // connected by |
}

ParsedExpression {
  // The top-level parse result
  pipelines: ParsedPipeline[]      // connected by &&, ;, ||
  operators: ("&&" | ";" | "||")[] // operators between pipelines
  redirects: Redirect[]
}

Redirect {
  type: ">" | ">>" | "<" | "2>" | "2>>" | "&>"
  target: string
}

// Interface
Parser {
  parse(input: string) -> ParsedExpression | ParseError
}
```

The key design choice: the parser produces a **command-oriented** view, not a raw shell AST. The classifier doesn't need to know about quoting rules, word splitting, or token types. It needs to know: what command, what flags, what's piped to what, are there redirects.

If we use mvdan/sh under the hood, this layer translates from mvdan's AST to our simpler types. If we swap parsers later, only this layer changes.

### Classifier

```
// Types
Classification = "read" | "write" | "unknown"

ClassifyResult {
  classification: Classification
  components: ComponentResult[]   // per-command breakdown
}

ComponentResult {
  command: string
  classification: Classification
  reason: string | null           // "grep is read-only", "-i flag makes sed write"
}

// Interface
Classifier {
  classify(input: string) -> ClassifyResult
}
```

The classifier is the integration point — it owns a Parser and a Database and coordinates between them. It handles:
- Parsing the input
- Looking up each command
- Applying flag effects
- Resolving subcommands
- Recursive evaluation (extracting inner commands, re-classifying)
- Composition rules (pipelines: worst-of, redirects: `>` makes it write)
- Returning the aggregate result

### Adapters

```
// Interface
Adapter {
  // Extract a command string from environment-specific input
  extract_command(input: bytes) -> string

  // Format a ClassifyResult into environment-specific output
  format_result(result: ClassifyResult) -> bytes
}

// Implementations
PlainAdapter    // command as CLI arg → human-readable text
JsonAdapter     // command as CLI arg → structured JSON
ClaudeCodeAdapter  // PreToolUse JSON on stdin → hookSpecificOutput JSON on stdout
```

Each adapter is thin — just input extraction and output formatting. All classification logic lives in the Classifier.

---

## Implementation Order (Test-Driven)

Each step: **write tests first**, then implement until tests pass.

### Step 1: Database Loader

**Tests:**
- Load a minimal TOML file, look up a command, get its definition
- Look up a command that doesn't exist → null
- Load a command with flags, verify flag definitions
- Load a command with subcommands, look up a subcommand
- Load a command with recursive fields
- Schema validation: reject malformed entries (missing required fields, invalid classification values)

**Implementation:**
- Define the TOML schema (finalize from doc 10)
- Write the loader
- Write the lookup function
- Seed 5-10 command definitions to test against (grep, sed, find, git, cat, ls, rm, cp, mv)

**Interface exposed:** `Database.lookup(command) -> CommandDef | null`

### Step 2: Parser

**Tests:**
- Parse simple command: `"grep -r TODO src/"` → `ParsedCommand{command: "grep", args: ["-r", "TODO", "src/"], flags: [{name: "-r"}]}`
- Parse command with subcommand: `"git log --oneline"` → subcommand = "log"
- Parse pipeline: `"cat file | grep error | sort"` → 3 ParsedCommands
- Parse compound: `"cmd1 && cmd2 ; cmd3"` → 3 pipelines with operators
- Parse redirects: `"echo hello > file.txt"` → redirect detected
- Parse command substitution: `"echo $(ls)"` → substitution detected
- Parse string arguments: `"sh -c 'grep foo bar'"` → string arg preserved for recursive extraction
- Edge cases: empty string, whitespace-only, unclosed quotes → ParseError

**Implementation:**
- Wrap mvdan/sh (or chosen parser) behind the Parser interface
- Translate mvdan's AST → our ParsedExpression types
- Handle the subset of shell syntax we care about, map everything else to a sensible default

**Interface exposed:** `Parser.parse(input) -> ParsedExpression`

### Step 3: Classifier — Basic

**Tests (from compliance test files):**
- Simple read command: `"grep -r TODO"` → read
- Simple write command: `"rm file.txt"` → write
- Unknown command: `"some-tool --flag"` → unknown
- Flag override: `"sed 's/foo/bar/' f"` → read, `"sed -i 's/foo/bar/' f"` → write
- Subcommand: `"git log"` → read, `"git push"` → write

**Implementation:**
- Wire Parser + Database together
- For each parsed command: look up in DB, check flags against flag definitions
- Return the "worst" classification: write > unknown > read

**Interface exposed:** `Classifier.classify(input) -> ClassifyResult`

### Step 4: Classifier — Composition

**Tests:**
- Pipeline (all read): `"cat file | grep error | sort"` → read
- Pipeline (one write): `"cat file | tee output.log"` → write
- Compound (all read): `"ls && pwd"` → read
- Compound (one write): `"ls && rm file"` → write
- Redirect: `"grep error file > output.txt"` → write (regardless of command)
- Redirect append: `"echo msg >> log.txt"` → write
- Command substitution: `"echo $(rm file)"` → write

**Implementation:**
- Walk all commands in a ParsedExpression
- Classify each independently
- Apply composition rules: worst-of for pipelines/compounds, any redirect → write
- Walk into command substitutions and classify those too

### Step 5: Classifier — Recursive Evaluation

**Tests:**
- Transparent wrapper: `"sudo ls -la"` → read
- Transparent wrapper: `"sudo rm -rf /"` → write
- Env wrapper: `"env FOO=bar grep TODO"` → read
- Exec flag: `"find . -exec cat {} \\;"` → read
- Exec flag: `"find . -exec rm {} \\;"` → write
- Separator: `"kubectl exec pod -- ls"` → read
- String recursion: `"sh -c 'grep foo bar'"` → read
- String recursion: `"sh -c 'rm -rf /'"` → write
- Chained: `"sudo env FOO=bar xargs grep TODO"` → read
- Depth limit: deeply nested wrappers → unknown after limit
- Extraction failure: `"xargs"` (no inner command) → unknown (falls back to default)

**Implementation:**
- When a command is marked `recursive` or a flag has `effect = "recursive"`, extract inner command using the strategy encoded in the DB
- Call classify recursively on the extracted command
- Enforce depth limit

### Step 6: CLI

**Tests (integration, against the compiled binary):**
- `kjell check "grep -r TODO"` → prints "read" (exit 0)
- `kjell check "rm file"` → prints "write" (exit 0)
- `kjell check --json "grep -r TODO"` → valid JSON with classification field
- `kjell explain "cat file | tee out.log"` → human-readable pipeline breakdown
- `kjell check --format claude-code < hook_input.json` → valid hookSpecificOutput JSON
- `kjell db lookup grep` → prints DB entry
- `kjell db validate` → validates all DB files

**Implementation:**
- CLI argument parsing
- Wire up adapters to the Classifier
- Exit codes: 0 = classified, 1 = error

### Step 7: Claude Code Adapter

**Tests:**
- Parse PreToolUse JSON input → extract command string
- Classify read command → `permissionDecision: "allow"`
- Classify write command → `permissionDecision: "ask"`
- Classify unknown command → `permissionDecision: "ask"`
- Malformed input → exit code 2 (blocking error per hook protocol)

**Implementation:**
- Read stdin, parse as JSON, extract `tool_input.command`
- Run through Classifier
- Map result to Claude Code's hook response format
- Specific exit code handling per Claude Code's hook protocol

### Step 8: Database Seeding

Not code — just TOML files. But test-driven: write the compliance test file first, then write the DB entry.

**Initial command set (prioritized by frequency in agent workflows):**

Batch 1 — ubiquitous reads:
`cat`, `ls`, `head`, `tail`, `less`, `wc`, `sort`, `uniq`, `diff`, `grep`, `find` (without -exec/-delete), `which`, `whereis`, `file`, `stat`, `du`, `df`, `pwd`, `echo`, `printf`, `date`, `whoami`, `hostname`, `uname`, `id`, `test`/`[`

Batch 2 — dev tools (read operations):
`git log/diff/status/branch/tag/show/remote`, `node --version`, `npm list/ls`, `cargo --version`, `go version`, `python --version`, `pip list`, `rustc --version`

Batch 3 — writes:
`rm`, `cp`, `mv`, `mkdir`, `touch`, `chmod`, `chown`, `ln`, `git commit/push/checkout/merge/rebase/reset`, `npm install/publish`, `pip install`

Batch 4 — flag-sensitive:
`sed` (±`-i`), `find` (±`-exec`/`-delete`), `curl` (±`-X`/`-d`), `tee`, `tar` (create vs list)

Batch 5 — recursive:
`sudo`, `env`, `nice`, `nohup`, `time`, `timeout`, `watch`, `xargs`, `sh`/`bash` (`-c`), `ssh`, `docker exec/run`, `kubectl exec`

For each: write the test file first (`tests/commands/grep.toml`), then the DB entry (`db/grep.toml`).

---

## Project Structure

Unit tests live next to their implementations (e.g., `database_test.go` next to `database.go`). Only integration/compliance tests — the language-independent TOML test cases — live in a separate `tests/` directory. Those TOML files aren't unit tests; they're the cross-implementation contract that any language port must pass.

```
kjell/
├── docs/                        # Architecture docs (what we have now)
├── db/                          # Command database (TOML files)
│   ├── cat.toml
│   ├── grep.toml
│   ├── git.toml
│   ├── sed.toml
│   ├── find.toml
│   └── ...
├── tests/                       # Integration / compliance tests only
│   ├── commands/                # Per-command compliance tests (TOML)
│   │   ├── cat.toml
│   │   ├── grep.toml
│   │   └── ...
│   ├── composition/             # Pipes, redirects, compounds
│   │   ├── pipes.toml
│   │   ├── redirects.toml
│   │   └── compound.toml
│   └── edge_cases/
│       ├── unknown.toml
│       ├── malformed.toml
│       └── adversarial.toml
├── internal/
│   ├── database/
│   │   ├── database.go          # Layer 1: DB loader + lookup
│   │   └── database_test.go     # Unit tests for DB loading, lookup, validation
│   ├── parser/
│   │   ├── parser.go            # Layer 2: Shell parser wrapper
│   │   └── parser_test.go       # Unit tests for parsing specific constructs
│   ├── classifier/
│   │   ├── classifier.go        # Layer 3: Classification logic
│   │   ├── classifier_test.go   # Unit tests for classification rules, recursion
│   │   └── compliance_test.go   # Loads tests/commands/*.toml and runs them
│   └── adapter/
│       ├── plain.go
│       ├── json.go
│       ├── claude_code.go
│       └── adapter_test.go      # Unit tests for input/output formatting
├── cmd/
│   └── kjell/
│       └── main.go              # CLI entrypoint
└── go.mod
```

---

## Language Choice

The integration point is a CLI binary that speaks stdin/stdout. No agent harness is going to add a library dependency for this — they'll shell out to `kjell` (Claude Code hook) or consume generated allowlists (Cursor). The "cross-language library" angle from earlier docs is premature and probably unnecessary.

If an agent harness ever wants to integrate at the library level, they'd reimplement the classifier in their own language against the compliance test suite. That's the point of the suite.

So the language choice is purely internal:

1. **Quality of shell parser** — the hard dependency that determines correctness
2. **Single binary distribution** — `brew install kjell` or download from GitHub releases, no runtime deps
3. **Our velocity**

**Decision: Go.**
- `mvdan/sh` is the best shell parser in any language — this alone is decisive
- Single static binary, cross-compiles trivially
- Fast enough without thinking about it
- Good enough contributor pool for a focused project like this

---

## What's NOT in Scope for v1

- User config/overrides (`.kjell.toml`)
- Multiple output formats beyond plain/json/claude-code
- Database metadata fields (resource, operation, prompt_hint)
- Allowlist generation (`kjell generate-allowlist`)
- WASM build
- Language bindings (Python, Rust, etc.)
- `kjell explain` command (nice-to-have, not core)
