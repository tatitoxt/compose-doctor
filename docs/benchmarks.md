# Benchmarks

End-to-end load benchmarks for `compose.Load` — discovery, YAML parsing,
interpolation, `include:`/`extends:` resolution and multi-file merge — measured
with Go's built-in benchmark runner (`-benchtime=2s -count=3`, best of 3 shown).

**How to reproduce**

```sh
go test -run '^$' -bench . -benchtime=2s -count=3 ./internal/compose/
```

**Results (2026-11-21, Intel Xeon VM, Go 1.26, 8 cores)**

| Project | Time per load | Loads/sec | Memory per load | Allocs |
|---|---:|---:|---:|---:|
| `valid-minimal` (1 service) | 28.4 µs | ~35,000 | 15 KB | 138 |
| `profiles` | 35.7 µs | ~28,000 | 19 KB | 217 |
| `multi-file` (base + override) | 47.0 µs | ~21,000 | 27 KB | 274 |
| `valid-complex` (anchors, env, extends) | 163 µs | ~6,100 | 140 KB | 1,057 |
| generated 200-service file | 1.31 ms | ~760 | 804 KB | 12,741 |

**Reading the numbers**

- A typical Compose project loads in tens of microseconds — linting adds
  negligible latency even inside CI pipelines that run it per commit.
- The 200-service synthetic project parses and merges in ~1.3 ms, so
  `compose-doctor` scales to monorepo-sized Compose files without batching.
- These numbers are machine-dependent; they exist as a reproducible
  regression baseline, not an absolute guarantee.
