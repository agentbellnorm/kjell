# Adoption Strategy

## Goal: Get Used by at Least One Major Agent Harness

The database is only valuable if it's consumed. This doc lays out the concrete path from "local project" to "integrated in real tools."

## Integration Feasibility by Agent

| Agent | Runtime hook? | Static allowlist gen? | Requires fork? |
|---|---|---|---|
| **Claude Code** | **Yes** — PreToolUse hooks, full JSON API | **Yes** — settings.json generation | No |
| **Cursor** | No | **Yes** — `~/.cursor/permissions.json` | No |
| **Windsurf** | No | Partial — VS Code settings | No |
| **Aider** | No | No | Yes |
| **Cline** | No | No | Yes |
| **OpenHands** | No (container-level only) | No | Custom image |
| **LangGraph** | **Yes** — `interrupt()` primitive | N/A (programmatic) | No |

**Claude Code is the clear first target.** It has the richest hook system, designed exactly for this use case.

## Dogfooding Path: Claude Code PreToolUse Hook

### How It Works

Claude Code's hook system lets you intercept any tool call with an external binary. For `Bash` tool calls, the hook receives the full command as JSON on stdin and returns an allow/deny/ask decision.

### Config (in `~/.claude/settings.json` or `.claude/settings.json`):

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "kjell check --format claude-code",
            "timeout": 5
          }
        ]
      }
    ]
  }
}
```

That's it. One line. No wrapper script, no jq, no glue.

### What happens under the hood:

`--format claude-code` means kjell natively speaks the Claude Code hook protocol:

1. **Input**: reads the PreToolUse JSON from stdin, extracts `tool_input.command`
2. **Classify**: runs the command through the database + parser
3. **Output**: returns the result in Claude Code's `hookSpecificOutput` format:

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "allow",
    "permissionDecisionReason": "kjell: grep -r is read-only"
  }
}
```

Decision mapping: `read` → `"allow"`, `write`/`unknown` → `"ask"` (fall through to normal prompting).

### Day 1 Dogfooding

1. Build `kjell` CLI with `check` command and `--format claude-code`
2. Add the one-line hook config to Claude Code settings
3. Use Claude Code normally — read-only commands auto-approve, write commands prompt as before
4. Every misclassification becomes a test case
5. Iterate on the database based on real usage

## Phase 1: Dogfood (You)

**Goal**: kjell works well enough for daily use with Claude Code.

- Build CLI with `check` command and `--format` flag
- Seed database with 50-100 most common commands
- Use it daily, fix misclassifications, add test cases
- Track: how many commands auto-approved? How many false positives/negatives?

## Phase 2: Share (Early Adopters)

**Goal**: Other Claude Code users can install and use it.

- Publish CLI via Homebrew / `go install`
- Write a one-line setup guide: "add this to your Claude Code settings.json"
- Post on relevant forums/communities (Claude Code GitHub discussions, r/ClaudeAI, Hacker News)
- Collect feedback and command coverage requests

## Phase 3: Expand Coverage (Other Agents)

**Goal**: Provide value beyond Claude Code.

### Cursor Integration
Generate `~/.cursor/permissions.json` from the database:
```bash
kjell generate-allowlist --format cursor > ~/.cursor/permissions.json
```
This gives Cursor users a richer, flag-aware allowlist than manually curating one.

### LangGraph Integration
LangGraph agents can shell out to kjell in their `interrupt()` logic:
```python
import subprocess, json

def approve_command(command):
    result = subprocess.run(["kjell", "check", "--json", command], capture_output=True, text=True)
    classification = json.loads(result.stdout)["classification"]
    if classification == "read":
        return {"action": "approve"}
    return interrupt({"command": command, "classification": classification})
```

## Phase 4: Upstream (Contribute to Agents)

**Goal**: Agent harnesses adopt kjell's database or library directly.

- Open PRs against Claude Code, Aider, etc. to replace their internal allowlists with kjell
- Offer the database as a standalone JSON artifact (no library dependency needed)
- The compliance test suite demonstrates correctness

## Key Insight: Two Products, Different Audiences

1. **The database** — for agent harness maintainers who want better command metadata
2. **The CLI/hook** — for individual users who want better permissions today, without waiting for their agent to adopt anything

The CLI/hook is the wedge. It provides immediate value and proves the concept. The database is the long-term play.

## What Makes This Adoptable

- **Zero-config integration** with Claude Code (one settings.json line)
- **Graceful degradation**: unknown commands fall through to normal prompting
- **Conservative by default**: never auto-approves something it doesn't recognize
- **Transparent**: `kjell explain <command>` shows exactly why a decision was made
- **Extensible**: users can add their own command definitions without forking

## What Could Kill Adoption

- **Too many false positives** (classifies reads as writes) → annoying, users disable it
- **Any false negatives** (classifies writes as reads) → security issue, trust destroyed
- **Too slow** → noticeable delay before every command
- **Hard to install** → needs to be a single binary, no runtime deps
- **Doesn't cover common commands** → users hit "unknown" too often and give up
