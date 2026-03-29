# Testability

## Testing Philosophy

The correctness of kjell is its entire value proposition. A wrong classification — especially classifying a destructive command as "read" — is a security issue. Testing must be thorough, automated, and shared across all implementations.

## Test Layers

### Layer 1: Shared Compliance Test Suite

The most important testing artifact. A language-independent set of test cases that any implementation must pass.

Format: a data file (TOML, JSON, or YAML) of `(input, expected_output)` pairs.

```toml
# tests/commands/basic.toml

[[tests]]
input = "ls -la"
expect = "read"

[[tests]]
input = "rm -rf /tmp/foo"
expect = "write"

[[tests]]
input = "grep -r 'TODO' src/"
expect = "read"

[[tests]]
input = "sed -i 's/foo/bar/' file.txt"
expect = "write"

[[tests]]
input = "sed 's/foo/bar/' file.txt"
expect = "read"
note = "sed without -i only writes to stdout"

[[tests]]
input = "find . -name '*.log' -delete"
expect = "write"

[[tests]]
input = "find . -name '*.log'"
expect = "read"

[[tests]]
input = "some-unknown-command --flag"
expect = "unknown"
```

This suite should be:
- **Comprehensive**: hundreds to thousands of test cases
- **Categorized**: by command, by feature (pipes, redirects, subshells), by edge case
- **Growing**: every bug report should become a test case
- **The contract**: if an implementation passes all tests, it's compliant

### Layer 2: Database Validation Tests

Ensure the database itself is well-formed:

- Every command entry has required fields
- Flag overrides reference valid flags
- Subcommand trees are consistent
- No contradictions (e.g., a command marked "always read" with a subcommand marked "always write" — that's fine, but the tool should flag it for human review)
- Cross-references are valid

### Layer 3: Parser Tests (Per Implementation)

Each language implementation tests its own parser:

- Tokenization of various shell constructs
- AST structure for pipes, redirects, subshells, command substitution
- Error handling for malformed input
- Edge cases: empty strings, only whitespace, extremely long commands

### Layer 4: Integration Tests

End-to-end tests that exercise the full pipeline:

- CLI: `kjell check "command"` → expected output
- Library: `classify("command")` → expected result struct
- Fuzz testing: random/malformed inputs should never crash, always return a result

### Layer 5: Real-World Corpus Testing

Collect actual commands from agent harnesses and test against them:

- Scrape command logs from Claude Code, Aider, etc. (with permission / from public traces)
- Classify each one and manually verify a sample
- Track coverage: what % of real-world commands does the database cover?
- Track accuracy: what % of classifications are correct?

## Fuzz Testing

Shell parsing is notoriously tricky. Fuzz testing should be a first-class concern:

- Feed random byte strings to the parser — it must never crash
- Feed syntactically valid but adversarial shell to the classifier — it must always return a result
- Use coverage-guided fuzzing (e.g., Go's built-in `go test -fuzz`)

## CI Strategy

- Compliance test suite runs on every PR that touches the database or any implementation
- Each language implementation has its own CI job
- Database validation runs on every PR that touches the database
- Fuzz testing runs nightly or weekly (not per-PR, too slow)
- Coverage tracking: aim for high coverage but don't obsess over a number

## Test Case Contribution Flow

Make it dead simple for the community to add test cases:

1. User encounters a misclassification
2. Opens an issue or PR with the command and expected classification
3. Test case is added to the compliance suite
4. All implementations must pass the new test before the next release

This is one of the most valuable community contribution paths — it requires no code, just knowledge of what commands do.
