# Database & Testing Structure

## Core Principle

The primary goal is: **agents never ask permission for read-only operations**. That's the 80% value. Everything else — rich classification, overrides, operation/resource taxonomies — is secondary and should not complicate the core.

---

## Part 1: Database Schema

Start with the simplest thing that solves the core problem.

### The Minimal Entry

```toml
# db/grep.toml
command = "grep"
default = "safe"
```

That's a valid entry. grep is read-only, no flags change that. Done.

### When Flags Matter

```toml
# db/sed.toml
command = "sed"
default = "safe"

[[flags]]
flag = ["-i", "--in-place"]
effect = "write"
reason = "Edits files in place"
```

### Subcommands

```toml
# db/git.toml
command = "git"

[subcommands.log]
default = "safe"

[subcommands.diff]
default = "safe"

[subcommands.status]
default = "safe"

[subcommands.commit]
default = "write"

[subcommands.push]
default = "write"

[subcommands.checkout]
default = "write"
```

### Classification Values

Keep it to the minimum that's useful:

```
read        — no side effects, safe to auto-approve
write       — modifies state, should prompt
unknown     — not in database, should prompt
```

That's it for v1. No "destructive" vs "write" distinction, no "network" dimension, no operation/resource taxonomy. An agent harness getting `read` auto-approves. Anything else prompts. Simple.

### Room to Grow (Without Breaking)

The schema should support richer classification *later* without breaking existing consumers. The way to do this: **extra fields are optional and ignored by simple consumers**.

```toml
# v1: this is all a consumer needs to look at
command = "kubectl"
[subcommands.apply]
default = "write"

# v2 (future): richer metadata, ignored by v1 consumers
[subcommands.apply.metadata]
resource = "kubernetes"
operation = "apply"
description = "Apply configuration to kubernetes cluster"
prompt_hint = "Apply kubernetes manifests"
```

A v1 consumer sees `default = "write"` and prompts. A future v2 consumer could use `prompt_hint` to show "Always allow applying kubernetes manifests?" instead of "Allow `kubectl apply -f deployment.yaml`?". But v1 doesn't need to know about any of that.

### The Open/Closed Question

The operation/resource model is interesting but premature for v1. Here's how to keep the door open:

**Closed**: The core classification (`read`/`write`/`unknown`) is fixed. Every consumer must understand these. This is the contract.

**Open**: The `metadata` section is a bag of optional key-value pairs. New keys can be added without a schema change. Consumers opt into reading specific keys. Examples of future metadata:

```toml
[subcommands.apply.metadata]
resource = "kubernetes"         # what kind of thing is being affected
operation = "apply"             # what's being done to it
prompt_hint = "Apply k8s manifests"  # human-readable for UX
network = true                  # touches network
reversible = false              # can't undo easily
```

No consumer is required to read any of these. The core `read`/`write`/`unknown` is always sufficient.

---

## Part 2: Agent UX Integration (Future, But Worth Designing For)

### The Problem With Current Permission Prompts

Claude Code today: "Allow `kubectl apply -f deployment.yaml`?" → User clicks "Always allow" → Saves glob `Bash(kubectl apply *)`.

This works but it's **command-pattern-based**. The user builds up their allowlist one command at a time by clicking through prompts.

### What kjell Could Enable

If kjell returns rich metadata, agent harnesses could present **semantic** permission prompts:

Instead of: "Allow `kubectl apply -f deployment.yaml`?"
Show: "Allow applying kubernetes manifests?" → Saves a rule that covers all `kubectl apply` variants.

Instead of: "Allow `git log --oneline -20`?"
Show: *(nothing — auto-approved because kjell says it's read-only)*

### How This Would Work With Claude Code

The PreToolUse hook can return `permissionDecisionReason` which is shown to the user. Today we'd use this for the classification explanation. But a future version could return `additionalContext` that Claude Code uses to generate better "always allow" rules:

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "ask",
    "permissionDecisionReason": "kjell: kubectl apply modifies kubernetes resources",
    "additionalContext": "This command applies kubernetes manifests. The user may want to allowlist all kubectl read operations."
  }
}
```

This is a **future integration** that depends on what Claude Code exposes. For now, the value is just: reads auto-approve, writes prompt. But the metadata field in the database schema means we can enrich the UX later without redesigning the database.

### Don't Build This Yet

The right order is:
1. Ship core read/write/unknown classification (v1)
2. Dogfood with Claude Code — see which prompts are annoying
3. Add metadata to the database entries that would help those specific prompts
4. Work with agent harness maintainers to surface that metadata in UX

---

## Part 3: Testing Structure

### One File Per Command, Mirrors the Database

```
db/
├── grep.toml
├── sed.toml
├── find.toml
├── git.toml
├── curl.toml
├── kubectl.toml
└── ...

tests/
├── commands/          # One test file per DB entry
│   ├── grep.toml
│   ├── sed.toml
│   ├── find.toml
│   ├── git.toml
│   ├── curl.toml
│   └── kubectl.toml
│
├── composition/       # Cross-command: pipes, redirects, compound
│   ├── pipes.toml
│   ├── redirects.toml
│   ├── compound.toml
│   └── substitution.toml
│
└── edge_cases/        # Malformed input, unknown commands, adversarial
    ├── unknown.toml
    ├── malformed.toml
    └── adversarial.toml
```

### Test Format

Keep it minimal. A test is an input and an expected classification.

```toml
# tests/commands/sed.toml

[[tests]]
input = "sed 's/foo/bar/' file.txt"
expect = "safe"

[[tests]]
input = "sed -i 's/foo/bar/' file.txt"
expect = "write"

[[tests]]
input = "sed --in-place 's/foo/bar/' file.txt"
expect = "write"

[[tests]]
input = "sed -i.bak 's/foo/bar/' file.txt"
expect = "write"
```

Optional `note` field for non-obvious cases:

```toml
[[tests]]
input = "sed -n 's/foo/bar/p' file.txt"
expect = "safe"
note = "-n suppresses output, -i not present, still read-only"
```

### Composition Tests

```toml
# tests/composition/pipes.toml

[[tests]]
input = "cat file.txt | grep error"
expect = "safe"

[[tests]]
input = "cat file.txt | tee output.log"
expect = "write"
note = "tee writes to a file"

[[tests]]
input = "grep error log.txt | sort | head -20"
expect = "safe"

[[tests]]
input = "echo hello > file.txt"
expect = "write"
note = "Redirect operator makes it a write regardless of command"

[[tests]]
input = "cat $(rm -rf /)"
expect = "write"
note = "Command substitution contains destructive command"
```

### CI Rules

Simple and enforceable:

1. Every `.toml` file in `db/` must have a corresponding file in `tests/commands/`
2. Every test file must have at least 2 test cases (the bare minimum: one read, one non-read — or just the default if no flags change it)
3. All tests pass in every implementation before merge
4. Database schema validation passes (required fields present, classifications are valid values)

### Test Contribution Is the Easiest Contribution

Lowering the bar:

- No code needed — just add a TOML entry
- Template in CONTRIBUTING.md: "Found a misclassification? Add a `[[tests]]` entry"
- GitHub issue template: "Command: ___, Expected: ___, Got: ___" → maintainer turns it into a test

---

## Part 4: User Overrides (Secondary)

Users can override or extend the database. Same format as the DB itself.

### Location

```
~/.config/kjell/overrides.toml     # User-level
.kjell.toml                        # Project-level
```

### Format

The DB uses one file per command (`db/grep.toml` with `command = "grep"` at the top level). Override files bundle multiple commands in one file using a `[commands.<name>]` table:

```toml
# .kjell.toml

# Add a custom internal tool
[commands.deploy-prod]
default = "write"

# Override a built-in
[commands.curl]
default = "write"   # "In this project, treat all curl as write"
```

Same fields as the DB entries, just namespaced under `[commands.*]`.

### Precedence

Project overrides > user overrides > built-in database. An override replaces the entire entry for that command (not a deep merge). This keeps the logic simple: look up command, check project overrides first, then user, then built-in.

### Not a Priority

Overrides exist so power users aren't blocked, not as a primary feature. The built-in database should be good enough that most users never need to override anything. If people are constantly overriding the same command, that's a signal to fix the built-in entry.
