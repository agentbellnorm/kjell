# Portability

## Cross-Platform Command Semantics

Shell commands differ across operating systems. The same command name can have different flags and behaviors.

### Dimension 1: OS Differences

| Aspect | Linux (GNU) | macOS (BSD) | Windows |
|--------|-------------|-------------|---------|
| `sed -i` | `sed -i 's/a/b/' file` | `sed -i '' 's/a/b/' file` (requires suffix arg) | N/A natively |
| `find` flags | GNU extensions (e.g., `-maxdepth` position) | Stricter POSIX ordering | N/A natively |
| `cp` flags | `--reflink`, `--sparse` | Missing GNU extensions | `copy` command |
| `ls` output | GNU coreutils format | BSD format | `dir` command |
| Package managers | `apt`, `yum`, `pacman` | `brew` | `choco`, `winget` |

### Strategy: Focus on POSIX + Common Extensions

1. **Primary target**: POSIX-defined behavior (the common subset)
2. **GNU extensions**: annotated as Linux-specific where they differ
3. **BSD variants**: annotated as macOS-specific where they differ
4. **Windows**: out of scope initially (agents on Windows typically use WSL, Git Bash, or similar POSIX-like environments)

### Dimension 2: Shell Differences

The parser needs to handle syntax differences across shells:

| Feature | bash | zsh | sh (POSIX) | fish |
|---------|------|-----|------------|------|
| Process substitution `<()` | Yes | Yes | No | No |
| Brace expansion `{a,b}` | Yes | Yes | No | Yes (different syntax) |
| `[[ ]]` tests | Yes | Yes | No | No |
| Array syntax | `arr=(a b)` | `arr=(a b)` | No | `set arr a b` |
| Globbing | Standard | Extended by default | Standard | Different |

### Strategy: Parse bash/zsh Superset

Most agent harnesses generate bash-compatible commands. We should:
1. Parse the bash/zsh superset of POSIX sh
2. Not attempt to parse fish, PowerShell, or other non-POSIX shells
3. Document which shell features are supported

### Dimension 3: Command Availability

Not all commands are available everywhere. The database should distinguish:
- **Ubiquitous**: `ls`, `cat`, `grep`, `find`, `cp`, `mv`, `rm`, `mkdir`, `chmod`, `echo`, `test`
- **Common but not universal**: `curl`, `wget`, `jq`, `git`, `docker`, `ssh`
- **Platform-specific**: `apt`, `brew`, `systemctl`, `launchctl`, `pbcopy`
- **Developer tools**: `node`, `npm`, `cargo`, `go`, `python`, `pip`

The database should tag commands with their typical availability so consumers can filter.

## Library Portability

### Build Targets
- Linux (x86_64, aarch64), macOS (x86_64, aarch64) — single static Go binary
- Windows (x86_64) — if there's demand, Go cross-compiles trivially

### Database Portability

The command database (TOML files) is language-independent:
- **Embedded** in the Go binary at build time (no runtime file loading)
- **Exportable** to JSON for consumption by other tools
- **Versionable**: semantic versioning so consumers can pin
- **Extensible**: user overrides without forking

If anyone wants a native library in another language, they reimplement against the compliance test suite. The database + test suite are the shared contract, not the Go code.

## Distribution

### As a CLI (primary)
- Homebrew (macOS/Linux)
- `go install`
- Pre-built binaries via GitHub releases

### As a Database Only (secondary)
- TOML files in the repo, plus JSON export
- For frameworks that want to do their own parsing/classification
