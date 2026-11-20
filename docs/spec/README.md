# Compose Doctor — MVP specification

## Problem

A developer who inherits or runs a local Docker Compose stack often loses time
in failed start-up loops: a variable that is not defined, an `env_file` that
does not exist, two services publishing the same host port, a service that
starts before its database is ready. Most of these problems are visible in the
files before anything is started.

**Job to be done:** "Before I start the whole stack, I want to detect likely
problems and get an explanation with file/line and a safe next step."

## Scope (MVP)

- `compose-doctor check [path]` reads Compose YAML files, applies offline,
  reproducible rules and reports findings with severity, location and a
  suggestion.
- Output formats: `human` (default) and `json` (stable, documented).
- `--strict` makes the exit code fail on configurable severities (for CI).
- Positive and negative fixtures for every rule.

Out of scope: replacing `docker compose config`, certifying security,
analysing Dockerfiles, Kubernetes, installing anything, `--fix`/autofix.

Hard constraints: never run containers, never invoke a shell or Docker, never
open network connections, never modify files, never print variable values.

## Acceptance scenarios

| # | Given | When | Then |
|---|-------|------|------|
| 1 | A valid Compose project | `check` | exit 0 and "No findings." (or justified advisories) |
| 2 | `${DB_PASSWORD}` referenced but not defined | `check` | finding with file and line; the value of no variable is ever printed |
| 3 | `env_file: ./missing.env` | `check` | error with the relative path and a hint to check name/location |
| 4 | Two services publish host port 8080 | `check` | error naming both services and both locations; ephemeral ports (`"80"`, `"127.0.0.1::80"`) never conflict |
| 5 | `depends_on` without `condition: service_healthy` | `check` | *info* suggestion, not an error |
| 6 | Invalid YAML | `check` | exit code 3 (distinct from findings), line/column when available, no panic |
| 7 | `--format json` | `check` | stable, parseable JSON; `--strict` fails only on configured severities |
| 8 | Any run | `check` | no network, no shell, no Docker, no file writes (tested) |
| 9 | Examples | — | synthetic values and `.env.example`; real `.env` is git-ignored |

Each scenario maps to tests; see `README.md#testing`.

## Supported Compose dialect

The Compose Specification as consumed by Docker Compose v2. The tool reads
only the keys used by its rules (see `docs/rules.md`); unknown keys are
ignored, not rejected. The legacy top-level `version` key is accepted and
reported as obsolete (`CD003`).

Supported loading features:

- File discovery in the project directory: `compose.yaml`, `compose.yml`,
  `docker-compose.yaml`, `docker-compose.yml` (first match), plus the
  matching `*.override.{yaml,yml}` file when present.
- Explicit files with repeated `-f` (later files override earlier ones).
- YAML anchors, aliases and merge keys (`<<: *anchor`).
- Interpolation: `$VAR`, `${VAR}`, `${VAR:-default}`, `${VAR-default}`,
  `${VAR:?msg}`, `${VAR?msg}`, `${VAR:+alt}`, `${VAR+alt}`, nested defaults
  and `$$` escaping.
- Variable sources: the process environment (unless `--ignore-os-env`) and the
  project `.env` file (or `--env-file`).
- Profiles: services with `profiles:` are analysed only when the profile is
  active (`--profile`, or `COMPOSE_PROFILES`), like Docker Compose.

See `docs/adr/` for the decisions behind these choices.
