# ADR 0003 — Severities and exit codes

Status: accepted

## Severities

- `error`: Docker Compose is expected to fail (or the service cannot work)
  with this configuration.
- `warning`: likely a mistake, but Compose may still start (e.g. an undefined
  variable becomes an empty string).
- `info`: advisory with limited certainty (e.g. `depends_on` without a
  readiness condition).

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Analysis completed; without `--strict`, findings never change the exit code |
| 1 | `--strict` and at least one finding at or above `--fail-on` (default `warning`) |
| 2 | Usage error (unknown flag, bad value) |
| 3 | Input could not be analysed: no Compose file found, unreadable file, invalid YAML, input limits exceeded |

Invalid input uses its own code so CI can distinguish "the project has
problems" from "the tool could not read the project". Findings that make
analysis impossible (`CD001`) are still printed in the requested format.
