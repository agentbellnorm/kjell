# Prior Art & Gap Analysis

## The Honest Question: Does This Need to Exist?

Before building anything, we need to be clear about what already exists and where the actual gap is.

## How Agent Harnesses Handle Permissions Today

### Claude Code (closed source, most sophisticated)
- **Glob pattern matching** on command strings: `Bash(npm run *)`, `Bash(git commit *)`
- Six permission modes from `plan` (read-only) to `bypassPermissions` (skip all)
- Understands `&&` and `;` — prefix rules don't accidentally approve compound commands
- "Don't ask again" saves separate rules per subcommand (up to 5)
- **Auto mode**: uses a secondary LLM to judge safety with prose-based rules
- **OS-level sandbox**: macOS Seatbelt, Linux Landlock/bubblewrap for filesystem/network
- **PreToolUse hooks**: arbitrary shell commands can block/allow at runtime
- **Key limitation**: string-pattern-based. Does NOT parse ASTs, does NOT understand flag semantics (`sed` vs `sed -i`), documentation warns argument patterns are "fragile"

### Aider
- Binary yes/no prompt before every command, or `--yes` to approve everything
- No allowlist, no classification, no analysis whatsoever

### OpenHands & SWE-agent
- **Container isolation**: commands run freely inside Docker/Fargate
- No per-command permission system at all
- Security = container boundary, not command classification

### Cursor
- OS-level sandbox (Seatbelt/Landlock). Flat allowlist of command prefixes to bypass sandbox.

### Windsurf
- Four levels: disabled → allowlist only → AI judges → auto-approve everything
- Allow/deny lists via VS Code settings. No AST parsing.

### Cline
- Explicit human approval for everything, or a toggle to approve everything. Nothing in between.

## Directly Relevant Existing Projects

### safecmd (Answer.AI) — github.com/AnswerDotAI/safecmd
**This is the closest thing to what kjell proposes.** It deserves detailed analysis.

What it does:
- Maintains categorized allowlists of safe commands (read-only utils, git read ops, package managers)
- Uses **AST parsing via `shfmt --tojson`** (the `bashxtract` module) to extract all commands from pipelines, subshells, command substitutions
- Validates at multiple levels:
  - Command name against allowlist
  - Denied flags per command (e.g., `find:-delete`)
  - Positional destination arguments (e.g., last arg of `cp`)
  - Output redirects (`>`, `>>` are blocked)
- Uses `CmdSpec` objects with `denied` flags, `exec_flags`, `dest_flags`, `exec_pos`/`dest_pos`
- Catches nested danger: `echo $(rm -rf /)`

What it doesn't do:
- Python-only, not usable from other languages
- Allowlist approach, not a semantic database (doesn't explain *why* a flag changes classification)
- No structured metadata per flag (just "this flag is denied")
- No categories beyond allow/deny
- No community contribution workflow or compliance test suite

**kjell's relationship to safecmd**: safecmd validates the approach (AST parsing + per-flag analysis works). kjell would build on this by making it a reusable database + library rather than a standalone Python tool.

### omamori — github.com/yottayoshida/omamori
macOS-only safety guard for AI agents:
- PATH shims for `rm`, `git`, `chmod`, `find`, `rsync` that intercept before execution
- Hooks that unwrap nested shell invocations (`sudo env bash -c "..."`)
- Context-aware: `rm -rf node_modules/` treated differently from `rm -rf src/`
- Transforms dangerous → safer alternatives (`rm -rf` → move to Trash)
- Self-defense against agents trying to disable it

**kjell's relationship to omamori**: different approach entirely. omamori intercepts at runtime (PATH shims), kjell classifies statically. Complementary, not competing.

## Shell Parsing Infrastructure

### mvdan/sh (Go) — 8.6k stars
- The best shell parser available in any language
- Powers `shfmt`, the standard shell formatter
- Full POSIX sh + bash + mksh + experimental zsh
- AST types directly useful for classification: `CallExpr`, `Redirect`, `BinaryCmd`, `CmdSubst`, `Subshell`
- Easy AST walking: `syntax.Walk(node, func(node syntax.Node) bool { ... })`
- **safecmd already uses shfmt's JSON AST output**, validating this as the right parsing infrastructure
- Note: `mvdan-sh` npm package is archived. Successor is `sh-syntax` npm package.

### tree-sitter-bash (C, with bindings everywhere)
- Part of tree-sitter ecosystem
- Incremental parsing, error recovery
- Bindings for Rust, Python, JS, Go
- Grammar is community-maintained
- Good but more oriented toward syntax highlighting than semantic analysis

### bash-parser (JavaScript) — less maintained
### bashlex (Python) — used by explainshell
### shell-words (Rust) — tokenizer only, no AST

### ShellCheck (Haskell, GPLv3)
- Has an AST (`T_SimpleCommand`, `T_Pipeline`, `T_Redirecting`, etc.)
- **Does NOT classify read/write.** Closest rule: SC2094 ("don't read and write the same file in a pipeline")
- AST is Haskell-only and tightly coupled to ShellCheck's analysis engine
- Not practical to reuse

## Command Metadata Projects
- **tldr-pages**: ~2000 commands with simplified docs. No read/write semantics.
- **explainshell**: Links commands to man page sections. Explanation, not classification.
- Nothing exists that structures per-command, per-flag read/write semantics.

## Where the Actual Gap Is

| What Exists | What's Missing |
|-------------|----------------|
| Ad-hoc allowlists per tool (Claude Code, Windsurf, safecmd) | A **shared, structured database** of command semantics any tool can consume |
| safecmd's per-flag denied flags | **Semantic annotations**: *why* a flag changes classification, what it does |
| safecmd's AST parsing via shfmt | A **reusable CLI tool** any agent can call |
| Binary allow/deny | **Three-way classification**: read, write, unknown |
| Container isolation (OpenHands, SWE-agent) | A **lightweight classification** for when containers are overkill |
| AI classifiers (Claude Code auto mode) | A **deterministic, auditable** classifier that doesn't cost tokens |

## What Would NOT Be Novel
- The concept of command allowlists
- AST parsing of shell commands (safecmd does this)
- Blocking output redirects
- Container isolation as a complement to classification

## What WOULD Be Novel
1. A structured, community-maintained database of per-command, per-flag, per-subcommand read/write semantics with human-readable reasons
2. A reusable CLI tool any agent can shell out to for fast classification
3. Composition-aware analysis that reasons about pipes/redirects/subshells as a graph
4. Multi-category classification beyond read/write binary
5. A shared compliance test suite that multiple implementations can validate against

## Key Insight from safecmd

safecmd already proved the core approach works. The AST parsing + per-flag analysis pattern is sound. The question for kjell isn't "can this work?" — it's "can we make it reusable, comprehensive, and community-maintained?"

## Risk: Is the Database Enough?

One legitimate concern: if the database is the main value, should this just be a data project (a JSON file on GitHub) rather than a library? Agent harnesses already have their own parsers and permission systems — maybe they just need better data.

Counter-argument: the parser logic (how to walk an AST and apply the database rules for pipes, redirects, subshells, flag combinations) is non-trivial and shouldn't be reimplemented by every consumer. The library wrapping the database provides:
- Correct AST walking (handling all the edge cases)
- Composition rules (conservative classification of pipelines)
- A testable contract (the compliance test suite)
- A standard API that simplifies integration

But the database-only distribution path should exist too, for consumers who want to do their own thing.
