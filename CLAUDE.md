# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is kjell?

kjell classifies shell commands as **safe**, **write**, or **unknown** for AI coding agents. It uses a proper bash parser (`mvdan.cc/sh/v3`) combined with a TOML database of 105+ command definitions to handle pipes, redirects, `&&`/`||`, command substitution, loops, conditionals, and recursive wrappers (`sudo`, `env`, `xargs`, `sh -c`, `find -exec`).

## Commands

```bash
go build ./cmd/kjell/          # Build
go test ./...                  # Run all tests (~319 tests)
go test ./internal/classifier/ # Run tests for a specific package
```

## Architecture

The pipeline is: **parser → classifier → adapter**

- **`cmd/kjell/main.go`** — CLI entry point with two subcommands: `check` (classify a command) and `db` (inspect the database). Handles `--json` and `--format` flags.
- **`internal/parser/`** — Parses shell strings into `ParsedExpression` trees using `mvdan.cc/sh`. Extracts command name, args, flags, subcommands, pipes, redirects, and operators.
- **`internal/classifier/`** — Classifies parsed commands against the TOML database. Key rules:
  - Pipelines use "worst case" semantics (if any stage is write, the whole pipeline is write)
  - Redirects to `/dev/null` are harmless; redirects to real files are write
  - Recursive commands (sudo, env, xargs, docker exec) evaluate the inner command up to depth 10
  - Flags can change classification (e.g., `curl -X GET` → safe, `curl -X POST` → write)
- **`internal/database/`** — Loads TOML command definitions from the embedded filesystem (`db.go` uses `go:embed`)
- **`internal/adapter/`** — Formats output as plain text, JSON, or Claude Code hook response (`claude-code` format auto-approves safe commands)
- **`db/`** — TOML files defining command classifications with flag-level and subcommand-level granularity

## Database TOML format

Each file in `db/` defines a command. Example structure:

```toml
command = "git"
default = "unknown"

[subcommands.log]
default = "safe"

[subcommands.push]
default = "write"
```

Flags can have value-dependent classifications via `[flags.values]` maps. Recursive commands use `recursive = true` with optional `separator` and `inner_command_position`.

## Tests

Test cases live in `tests/` as TOML files organized into `commands/`, `composition/`, and `edge_cases/`. The compliance test runner in `internal/classifier/compliance_test.go` loads all test TOMLs automatically. Format:

```toml
[[tests]]
input = "sudo cat /etc/hosts"
expect = "safe"
note = "recursive evaluation through sudo"
```
