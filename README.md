# kjell

AI coding agents execute shell commands but can't tell which ones are safe. kjell solves this — it parses shell commands and classifies them as **read**, **write**, or **unknown** so agents can auto-approve reads and only prompt for writes.

```bash
$ kjell check "cat logs/*.txt | grep error | sort"
READ

$ kjell check "sed -i 's/foo/bar/g' config.yaml"
WRITE

$ kjell check "sudo env CI=1 bash -c 'find /tmp -name *.cache -delete'"
WRITE
```

It handles pipes, redirects, `&&`/`||`, command substitution, loops, conditionals, and recursive wrappers (`sudo`, `env`, `xargs`, `sh -c`, `find -exec`, `docker exec --`). 100+ commands with flag-level granularity. Proper bash parser, not regex.

## Claude Code

Add to `.claude/settings.local.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "kjell check --format claude-code"
          }
        ]
      }
    ]
  }
}
```

Read-only commands auto-approve. Writes and unknowns pass through to Claude Code's normal permission system — your "always allow" rules still work.

## Install

```bash
brew install agentbellnorm/tap/kjell
```

Or with Go:

```bash
go install github.com/agentbellnorm/kjell/cmd/kjell@latest
```

Or grab a binary from [GitHub Releases](https://github.com/agentbellnorm/kjell/releases).

## CLI

```bash
kjell check "grep -r TODO src/"         # READ
kjell check --json "rm -rf /tmp/junk"   # JSON output
kjell db stats                           # show DB size
kjell db lookup git                      # show a command's entry
```

## Adding commands

Create a TOML file in `db/`:

```toml
command = "mycommand"
default = "read"

[[flags]]
flag = ["-w", "--write"]
effect = "write"
reason = "Modifies files"
```

Tests go in `tests/commands/` in the same format. Run with `go test ./...` (319 tests).
