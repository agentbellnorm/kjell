# Problem Statement

## The Core Problem

AI coding agents need to execute shell commands as part of their workflow, but determining whether a command is **read-only** (safe to auto-approve) or **has side effects** (needs user confirmation) is unsolved in a general, reusable way.

Today, every agent harness independently maintains its own coarse-grained allowlists. This leads to:

1. **Excessive permission prompts** — agents ask for approval on obviously safe commands like `ls`, `cat`, `grep`, breaking flow and wasting user time.
2. **Duplicated effort** — Claude Code, Aider, OpenHands, SWE-agent, and others each maintain their own command allowlists with slightly different coverage and semantics.
3. **Coarse granularity** — current allowlists operate at the command level, not the flag level. `sed` is either allowed or blocked, even though `sed` (no `-i`) is read-only while `sed -i` modifies files.
4. **No composability** — piped commands, redirections, and subshells aren't analyzed. `grep foo file.txt` is safe, but `grep foo file.txt > output.txt` writes. Current systems can't distinguish these.

## Why This Is Hard

Shell commands are Turing-complete. In the general case, statically determining whether a command has side effects is undecidable. Specific challenges:

- **Command substitution**: `cat $(rm -rf /)` looks like a read but isn't
- **Pipes to destructive commands**: `cat file | tee /etc/passwd`
- **Dynamic command construction**: `eval "$USER_INPUT"`
- **Exec flags**: `find . -exec rm {} \;`
- **Aliases and functions**: `alias cat='rm -rf'` (in the user's shell config)
- **Implicit side effects**: some commands have surprising write behaviors (e.g., `ssh` can forward ports, `curl` can POST)

## Why It's Tractable Anyway

The key insight is: **we don't need to solve bash in general**. We need to solve the commands that coding agents actually use. This is a much smaller, enumerable set.

- Agents typically use 50-100 common commands
- The commands are usually straightforward, not adversarial shell golf
- Flag-level semantics are well-documented and stable across versions
- The long tail of unknown commands can fall back to user prompting

## What We're Building

**kjell** — a shared, community-maintained database of shell command read/write semantics, with a fast analyzer that parses shell command strings and classifies them as read-only, write, or unknown.

The database encodes:
- Per-command default classification (read/write/unknown)
- Per-flag overrides (e.g., `sed` is read, `sed -i` is write)
- Subcommand semantics (e.g., `git log` is read, `git commit` is write)
- Composition rules (how pipes, redirects, and subshells affect classification)

The analyzer:
- Parses shell command ASTs (not regex matching on strings)
- Walks the AST and classifies each component against the database
- Returns a conservative classification: if any component is write/unknown, the whole pipeline is write/unknown
- Is fast enough for interactive use (sub-millisecond for typical commands)

## Success Criteria

1. Correctly classify >95% of commands that coding agents encounter in practice
2. Zero false negatives for common destructive operations (never auto-approve a write)
3. Usable as a CLI from any agent harness (stdin/stdout integration)
4. Community-maintainable database with clear contribution guidelines
5. Fast enough to be imperceptible (<5ms for typical commands)
