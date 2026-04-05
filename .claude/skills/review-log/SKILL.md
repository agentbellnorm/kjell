---
name: review-log
description: Read ~/.kjell/log, find commands classified as unknown or not-in-database, and recommend database additions
disable-model-invocation: true
---

Review the kjell classification log and recommend database improvements.

## Steps

1. Read `~/.kjell/log`. If it doesn't exist, tell the user to enable logging with `--log` and stop.

2. Parse the log. Each line is a structured `slog` entry with fields: `time`, `level`, `msg`, `command`, `classification`, `reason`. Extract all entries.

3. Group commands by their base command name (the first word in the `command` field). Count occurrences and note the classification(s) seen. Ignore one-off entries that look like user scripts or full paths (e.g., `/path/to/script.sh`).

4. For each command, check whether a TOML file already exists in the `db/` directory. The file may be named `<command>.toml` or `<command>_cmd.toml`. Read it if it exists to see what subcommands and defaults are defined.

5. Review for two kinds of issues:

   **A. Unknown / missing commands** — commands classified as `unknown` because they are not in the database or lack a subcommand entry.

   **B. Correctness concerns** — commands that *were* classified (`safe` or `write`) but where the classification may be wrong. For example:
   - A command classified as `safe` that can actually modify files or state
   - A command classified as `write` that is actually read-only in the way it was invoked
   - A default classification that is too permissive or too restrictive given common usage

6. Present a summary table:

   | Command | Count | Current Classification | Correct? | Recommendation |
   |---------|-------|----------------------|----------|----------------|

   For each command, recommend one of:
   - **Add to DB** — well-known CLI tool not yet in the database. Suggest a default classification and any subcommands if applicable.
   - **Add subcommand** — the base command exists but a frequently-seen subcommand is missing.
   - **Fix classification** — the existing classification is wrong. Explain what it should be and why.
   - **OK** — the current classification is correct, no action needed.
   - **Skip** — the command is a user script, one-off, or inherently unclassifiable (like `go run` which executes arbitrary code).

   Only include commands with a recommendation of Add, Fix, or Skip in the table — omit the OK ones to keep it concise.

7. For each "Add to DB", "Add subcommand", or "Fix classification" recommendation, show the exact TOML that should be added or modified, following the format used in existing `db/*.toml` files.

8. Ask the user which recommendations to apply. Only create or edit TOML files after confirmation.

## Classification guidance

- Read-only tools (linters, formatters that only print, info commands): `safe`
- Tools that modify files, state, or have side effects: `write`
- Tools that could do either depending on arguments, or execute arbitrary code: `unknown`
- When in doubt, prefer `unknown` over `safe` — it's safer to prompt than to auto-approve
