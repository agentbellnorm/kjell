# kjell

AI coding agents execute shell commands but can't tell which ones are safe. kjell solves this — it parses shell commands and classifies them as **safe**, **write**, or **unknown** so agents can auto-approve safe commands and only prompt for writes.

```bash
# Simple commands
$ kjell check "grep -r TODO src/"                              # SAFE
$ kjell check "rm -rf /tmp/junk"                               # WRITE

# Pipes — worst-case semantics
$ kjell check "cat logs/*.txt | grep error | sort"             # SAFE
$ kjell check "cat /etc/hosts | sed 's/old/new/' | tee hosts"  # WRITE (tee writes)

# Chained commands
$ kjell check "cd src && grep -r TODO ."                       # SAFE
$ kjell check "git add -A && git commit -m 'update'"           # WRITE

# Redirects
$ kjell check "ls -la 2>/dev/null"                             # SAFE (/dev/null is harmless)
$ kjell check "echo 'data' > output.txt"                       # WRITE (redirect to file)

# Flag-dependent classification
$ kjell check "curl -s https://api.example.com/status"         # SAFE (GET)
$ kjell check "curl -X POST https://api.example.com/deploy"    # WRITE (POST)

# Recursive wrappers — evaluates the inner command
$ kjell check "sudo cat /etc/hosts"                            # SAFE
$ kjell check "sudo rm -rf /var/cache"                         # WRITE

# kubectl exec — evaluates the command inside the container
$ kjell check "kubectl exec my-pod -- cat /etc/hosts"          # SAFE
$ kjell check "kubectl exec my-pod -- rm -rf /tmp/cache"       # WRITE

# Shell loops
$ kjell check 'for f in *.log; do grep error "$f"; done'       # SAFE
$ kjell check 'for f in *.log; do rm "$f"; done'               # WRITE

# Deep nesting — sudo → env → bash -c → actual command
$ kjell check "sudo env CI=1 bash -c 'grep -r TODO src/'"     # SAFE
$ kjell check "sudo env CI=1 bash -c 'find /tmp -name *.cache -delete'" # WRITE

# Python -c — analyzes the Python code
$ kjell check "python3 -c 'import json; json.loads(\"{}\")'"  # SAFE (pure computation)
$ kjell check "python3 -c 'import os; os.remove(\"foo\")'"    # WRITE (file deletion)
$ kjell check "python3 -c 'import os; os.system(\"ls -la\")'" # SAFE (shell recursion)
$ kjell check "python3 -c 'open(\"f.txt\", \"w\").write(\"x\")'" # WRITE (file write)
$ kjell check "python3 script.py"                              # UNKNOWN (can't analyze)
```

It handles pipes, redirects, `&&`/`||`, command substitution, loops, conditionals, recursive wrappers (`sudo`, `env`, `xargs`, `sh -c`, `find -exec`, `docker exec --`), and `python -c` code analysis. 100+ commands with flag-level granularity. Proper bash parser, not regex.

## Claude Code

Add to `.claude/settings.json`:

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

Safe commands auto-approve. Writes and unknowns pass through to Claude Code's normal permission system — your "always allow" rules still work.

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
kjell check "grep -r TODO src/"         # SAFE
kjell check --json "rm -rf /tmp/junk"   # JSON output
kjell check --log "kubectl get pods"    # classify and log to ~/.kjell/log
kjell db stats                           # show DB size
kjell db lookup git                      # show a command's entry
```

Use `--log` to write a classification trace to `~/.kjell/log` — useful for reviewing what kjell approved during an agent session:

```
time=2026-04-05T14:22:01Z level=INFO msg=classified command="cat /etc/hosts" classification=safe reason="cat: default safe"
time=2026-04-05T14:22:01Z level=INFO msg=classified command=sudo classification=safe reason="sudo wraps: cat /etc/hosts"
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

## Local overrides

Drop TOML files in `~/.kjell/db/` to add or override command definitions without modifying the built-in database. Files use the same format as above and are merged by command name — your local definitions take precedence.
