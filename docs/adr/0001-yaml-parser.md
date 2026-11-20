# ADR 0001 — Parse Compose files with `gopkg.in/yaml.v3` nodes

Status: accepted

## Context

Findings must point to a file, line and column. The reference implementation
(`compose-spec/compose-go`) produces a fully normalised project but drops
source positions, pulls in a large dependency tree and resolves things (for
example, relative paths and environment) in ways we need to observe rather
than apply.

## Decision

Decode each file into a `yaml.Node` tree with `gopkg.in/yaml.v3` (pinned at
v3.0.1) and build our own small, normalised model that keeps a location for
every element used by a rule. Only the subset of keys used by rules is
modelled; everything else is ignored.

Anchors, aliases and `<<` merge keys are resolved by our walker with a depth
limit. Input is bounded: 4 MiB per file and a nesting depth of 100.

## Consequences

- Precise locations and no transitive dependencies besides yaml.v3.
- We do not validate the full Compose schema; `docker compose config` remains
  the authority. Rules are written so that unknown/unsupported constructs are
  skipped instead of producing false positives.
- Merge semantics across multiple files are implemented for the modelled keys
  only (documented in `docs/rules.md`).
