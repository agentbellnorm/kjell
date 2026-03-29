# Use Cases

## 1. Agent Harness Integration

### Description
Agent harnesses (Claude Code, Cursor, etc.) call kjell as a CLI tool to classify shell commands before execution. The integration is stdin/stdout — the harness shells out to `kjell`, not imports it as a library.

### Flow
```
LLM generates command → harness intercepts → shells out to kjell → auto-approve / prompt user
```

### Requirements
- **CLI integration**: stdin/stdout, environment-specific formats via `--format`
- **Latency**: <5ms classification — imperceptible given LLM wait times
- **Conservative by default**: unknown commands must classify as "needs approval", never auto-approved
- Agent harnesses map kjell's output to their own permission model:
  - `read` → auto-approve
  - `write` / `unknown` → prompt user

### Agent-Specific Considerations
- Agents may chain commands with `&&`, `;`, `||` — all must be classified
- Agents sometimes generate multi-line scripts — need to handle heredocs, line continuations
- Some harnesses run commands in containers/sandboxes — kjell classification can inform whether sandboxing is even needed
- Harnesses may want to maintain their own additional allowlists on top of kjell's database


## 2. CLI Use

### Description
A standalone command-line tool for developers and security teams to classify commands interactively, in scripts, or as a pre-execution hook.

### Subcommands

#### `kjell check <command>`
Classify a single command. Default output is human-readable.
```bash
$ kjell check "find . -name '*.log' -delete"
WRITE
  find: read-only by default
  -delete flag: makes find delete matching files

$ kjell check "git log --oneline -20"
READ
  git log: read-only subcommand
```

#### `kjell check --json <command>`
Machine-readable output for scripting.
```json
{
  "input": "find . -name '*.log' -delete",
  "classification": "write",
  "components": [
    {
      "command": "find",
      "classification": "write",
      "reason": "-delete flag makes find delete matching files"
    }
  ]
}
```

#### `kjell check --format <environment>` — Environment-Aware I/O

The key insight: different agents have different hook protocols. Rather than making users write glue scripts, kjell should natively speak each agent's wire format for both **input** (reading the hook payload) and **output** (returning the decision in the expected shape).

```bash
# Claude Code: reads PreToolUse JSON from stdin, returns hookSpecificOutput JSON
$ kjell check --format claude-code
# Reads: {"tool_name":"Bash","tool_input":{"command":"grep -r TODO src/"}, ...}
# Returns: {"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow",...}}

# Plain: just classify a command string, human output (default)
$ kjell check "grep -r TODO src/"
# Returns: READ

# JSON: structured output, not tied to any agent
$ kjell check --json "grep -r TODO src/"
# Returns: {"classification":"read",...}

# Cursor: generate a permissions.json fragment
$ kjell check --format cursor
```

**Supported formats:**

| Format | Input | Output | Use Case |
|--------|-------|--------|----------|
| `plain` (default) | command as argument | human-readable text | Interactive use |
| `json` | command as argument | structured JSON | Scripting, piping |
| `claude-code` | PreToolUse hook JSON on stdin | `hookSpecificOutput` JSON on stdout | Claude Code hook |
| `cursor` | command as argument | permissions.json fragment | Cursor allowlist |

This means Claude Code integration is literally one line in settings.json:
```json
{
  "hooks": {
    "PreToolUse": [{
      "matcher": "Bash",
      "hooks": [{"type": "command", "command": "kjell check --format claude-code"}]
    }]
  }
}
```

No wrapper script. No jq. No glue. kjell understands the protocol directly.

#### Adding New Formats

New agent formats are added as the ecosystem evolves. Each format is a thin adapter:
- **Input adapter**: extracts the command string from the agent's hook payload
- **Output adapter**: wraps the classification result in the agent's expected response shape
- **Mapping**: translates kjell's classification categories to the agent's permission model (e.g., Claude Code has allow/deny/ask, others may differ)

```bash
# Future: as more agents add hook systems
$ kjell check --format aider      # if/when aider adds hooks
$ kjell check --format windsurf   # if/when windsurf adds hooks
$ kjell check --format langchain  # for LangGraph interrupt() protocol
```

#### `kjell explain <command>`
Human-readable breakdown of why a command got its classification.
```bash
$ kjell explain "cat file.txt | tee output.log | grep error"
Pipeline classification: WRITE

Step-by-step:
  1. cat file.txt         → READ (reads file to stdout)
  2. tee output.log       → WRITE (writes stdin to both stdout AND output.log)
  3. grep error           → READ (filters stdin)

The pipeline is WRITE because tee writes to output.log.
Without tee, this pipeline would be READ.
```

#### `kjell db`
Database management commands.
```bash
$ kjell db stats          # Show database coverage statistics
$ kjell db lookup grep    # Show the database entry for grep
$ kjell db validate       # Validate the database for consistency
$ kjell db export --format json  # Export database for external use
```

### Use as a Shell Hook
```bash
# In .bashrc or .zshrc — warn before running write commands
preexec() {
  result=$(kjell check --json "$1" 2>/dev/null)
  classification=$(echo "$result" | jq -r '.classification')
  if [ "$classification" = "write" ]; then
    echo "WARNING: This command is classified as write."
    read -p "Continue? [y/N] " confirm
    [ "$confirm" != "y" ] && return 1
  fi
}
```
