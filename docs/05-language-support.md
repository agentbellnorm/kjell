# Supporting Different Languages

## The Consumption Landscape

Agent harnesses are written in various languages. For kjell to be broadly useful, it needs to be consumable from at least the major ones:

| Harness | Language | Notes |
|---------|----------|-------|
| Claude Code | TypeScript | Most likely early adopter |
| Aider | Python | Popular open-source agent |
| OpenHands | Python | Research-oriented |
| SWE-agent | Python | Research-oriented |
| Cursor | TypeScript | IDE-integrated |
| Goose | Go + Python | Modular architecture |
| Custom agents | Varies | Rust, Go, TS, Python |

## Architecture Options

### Option A: Native Core + FFI Bindings

Write the core in a systems language (Rust or Go), expose a C ABI, generate bindings for other languages.

```
                    ┌─────────────┐
                    │  Rust Core  │
                    └──────┬──────┘
                           │ C ABI / FFI
              ┌────────────┼────────────┐
              │            │            │
        ┌─────┴─────┐ ┌───┴───┐ ┌─────┴─────┐
        │ Python     │ │ Node  │ │ Go        │
        │ (PyO3/    │ │(napi- │ │ (CGo)     │
        │  cffi)    │ │ rs)   │ │           │
        └───────────┘ └───────┘ └───────────┘
```

**Pros**: single source of truth for logic, high performance, one set of tests for core behavior
**Cons**: FFI complexity, build toolchain complexity (cross-compilation), harder for contributors who don't know the core language

### Option B: Shared Database + Per-Language Implementations

The database (command definitions) is shared data (JSON/TOML). Each language has its own parser and classifier that reads the same database.

```
        ┌─────────────────────────┐
        │ Shared Database (TOML)  │
        └────────────┬────────────┘
                     │ loaded by
        ┌────────────┼────────────┐
        │            │            │
   ┌────┴────┐ ┌────┴────┐ ┌────┴────┐
   │ Rust    │ │ TS      │ │ Python  │
   │ impl    │ │ impl    │ │ impl    │
   └─────────┘ └─────────┘ └─────────┘
```

**Pros**: each implementation is idiomatic, no FFI complexity, easier for community contributions in each language, database is the shared contract
**Cons**: multiple implementations to maintain, risk of behavioral divergence between languages, testing burden multiplied

### Option C: WASM Core + Native Wrapper

Compile core to WASM. Each language loads the WASM module.

```
        ┌───────────────┐
        │  Core (Rust)  │
        └───────┬───────┘
                │ compiled to
        ┌───────┴───────┐
        │     WASM      │
        └───────┬───────┘
                │ loaded by
   ┌────────────┼────────────┐
   │            │            │
   │ wasmtime   │ wasm in    │ wasm in
   │ (Rust/Go/  │ Node.js    │ Python
   │  Python)   │            │
   └────────────┘            │
```

**Pros**: single implementation, works everywhere WASM runs, no C ABI complexity
**Cons**: WASM runtime overhead (though small), limited access to host OS, harder to debug, some languages have immature WASM runtimes

### Option D: TypeScript Core + Transpile/Port

Write the core in TypeScript (since many harnesses are TS/JS). Port to other languages as needed.

**Pros**: largest potential contributor pool, runs everywhere Node runs, simplest initial setup
**Cons**: performance ceiling, porting to Rust/Go is manual, no natural FFI story

## Decision: Go

The integration point is a CLI binary (stdin/stdout). Agent harnesses shell out to it (Claude Code hooks) or consume its output (allowlist generation). No harness will take a library dependency. So the language choice is purely internal.

**Go** wins because:
1. `mvdan/sh` is the best shell parser in any language — this is decisive
2. Single static binary, trivial cross-compilation and distribution
3. Simple language, easy for contributors

The cross-language library story from Options A/C is unnecessary. If an agent harness ever wants a native library, they'd reimplement against the compliance test suite (language-independent TOML files). That's the point of the suite.

Multi-language ports may happen later, but the reference implementation is Go.
