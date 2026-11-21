# ADR 0002 — Interpolation without disclosing values

Status: accepted

## Context

Rules need resolved values (a host port may be `${WEB_PORT:-8080}`), but the
variables often hold secrets. The tool's output is pasted into CI logs and
issues.

## Decision

- Interpolation is implemented in `internal/compose/interp.go` following the
  Compose syntax (`$VAR`, `${VAR}`, `:-`, `-`, `:?`, `?`, `:+`, `+`, `$$`).
- Values come from the process environment (opt-out with `--ignore-os-env`)
  and the project `.env` file. `env_file:` entries are **not** an
  interpolation source, matching Docker Compose; when a missing variable is
  found in one, the suggestion says so.
- Values live only in memory. Findings carry variable *names*, file paths and
  the raw (uninterpolated) text of the YAML scalar. When a port finding
  involves an interpolated scalar, the resolved port number is not printed.
- A test asserts that no value from any fixture `.env` file appears in any
  output.

## Consequences

Some messages are less specific ("both publish the same host port") when the
port comes from a variable. This is intentional.
