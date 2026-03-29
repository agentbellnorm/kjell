# Performance

## Performance Context

Classification sits between LLM output and command execution. Users are already waiting seconds-to-minutes for LLM responses — a few milliseconds for classification is completely fine. We should optimize for **correctness and maintainability first**, and only chase microseconds if profiling shows a real problem.

### Targets

| Operation | Target | Notes |
|-----------|--------|-------|
| Simple command (`ls -la`) | <5ms | Comfortable margin |
| Piped command (`cat f \| grep x \| sort`) | <10ms | Still imperceptible |
| Complex script (10+ lines) | <20ms | Rare in practice |
| Database initialization | <50ms | One-time cost at startup |

These are generous targets. In practice, we'll likely be well under them, but there's no reason to prematurely optimize.

### What Matters More Than Raw Speed

1. **Correctness**: a wrong classification (especially a false "read" for a write command) is infinitely worse than a slow correct one
2. **Startup time**: for CLI use, cold start should feel instant (<100ms total)
3. **Memory footprint**: the library shouldn't bloat the host process — a few MB is fine
4. **No pathological cases**: even adversarial inputs (deeply nested subshells, very long pipelines) should complete in bounded time, not hang

## Data Structure Choices

### Command Database

The database is small (~100-200 commands with their flags/subcommands). Almost any reasonable data structure works. Options:

- **HashMap**: simple, fast, good default
- **Perfect hash map** (compile-time): slightly faster lookups, but locks database to compile time
- **Just a JSON/TOML blob parsed at init**: simplest to maintain and contribute to

**Recommendation**: HashMap loaded from an embedded data file at initialization. Simple, fast enough, and easy to extend. Perfect hashing is a premature optimization we can add later if needed.

### Shell Parser

This is the more consequential choice. We're using `mvdan/sh` in Go (see `05-language-support.md`, `08-dependencies.md`). The key tradeoff was:

- **Use an existing parser** (tree-sitter-bash, bash syntax libraries): battle-tested grammars, less work, but we take on a dependency
- **Hand-roll a parser**: full control, no deps, but significant effort and potential for bugs in edge cases
- **Hybrid**: simple tokenizer for the common `command [flags] [args]` case, delegate to a full parser for pipes/redirects/subshells

We're using `mvdan/sh` in Go — see `05-language-support.md`.

## Memory

- Database: ~200KB estimated (100+ commands × subcommands × flags)
- Per-classification: one AST + result struct, a few KB at most
- No persistent state between classifications — stateless and thread-safe

## Things to NOT Optimize Early

- Sub-microsecond lookups (HashMap is fine)
- Zero-allocation paths (a few small allocations per classify call are fine)
- Incremental parsing (we're not editing shell scripts in real-time)
- SIMD or other low-level tricks

## Things Worth Doing From the Start

- **Bounded execution**: set a max parse depth / command length to prevent pathological inputs from causing issues
- **Lazy evaluation in pipelines**: if any component is classified "write", the whole pipeline is "write" — no need to continue analyzing
- **Benchmark suite with real-world commands**: collect actual commands from agent harnesses and use them as the benchmark corpus. This keeps us honest about what matters.
