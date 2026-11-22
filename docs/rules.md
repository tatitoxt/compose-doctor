# Rules

Every finding has a stable rule ID. Run `compose-doctor rules` (or
`rules --format json`) to list them from the binary. Severities are explained
in [ADR 0003](adr/0003-severity-and-exit-codes.md). Each rule is covered by
table tests in `internal/rules/*_test.go` (positive and false-positive cases)
and by at least one fixture in `testdata/fixtures/`.

| ID | Name | Severity | What it reports |
|----|------|----------|-----------------|
| CD001 | invalid-input | error | A Compose file is missing, unreadable, empty, not valid YAML or exceeds the input limits (4 MiB, nesting depth 100). Analysis stops; exit code 3. Cannot be disabled. |
| CD002 | service-without-image-or-build | error | A service has neither `image` nor `build` (services using `extends` are skipped). |
| CD003 | obsolete-version-key | info | The top-level `version` key, ignored by Docker Compose v2. Removed in place by `check --fix`. |
| CD004 | duplicate-key | error | The same key twice in one mapping; Docker Compose rejects the file. |
| CD101 | env-file-not-found | error | A required `env_file` is missing. Optional entries (`required: false`) are skipped; a sibling `*.example` file is suggested as a template. |
| CD102 | build-context-not-found | error | A local build context directory, or its `dockerfile`, does not exist. Remote contexts (URLs, `git@…`) and `dockerfile_inline` are skipped. |
| CD103 | bind-source-not-found | warning | A relative bind-mount source (`./…`, `../…`) does not exist. Absolute host paths are not checked because they depend on the machine that runs the stack. |
| CD104 | watch-path-not-found | warning | A relative `develop.watch` `path` does not exist; `docker compose watch` would only sync paths that are present. Absolute paths are skipped like bind sources. |
| CD201 | undefined-variable | warning | `$VAR`/`${VAR}` without a default whose variable is not set in the shell environment or the project `.env`. Reported once per variable. Points out when the name only exists in an `env_file` (which does not feed interpolation) or in `.env.example`. |
| CD202 | required-variable-unset | error | `${VAR:?msg}` / `${VAR?msg}` whose variable is unset (or empty for `:?`). |
| CD203 | invalid-interpolation | error | Malformed interpolation such as an unclosed `${`. |
| CD301 | host-port-conflict | error | Two mappings bind the same host IP, port and protocol. `0.0.0.0`/`::`/no IP overlaps every address. Ephemeral host ports, different protocols, different specific IPs, services with `network_mode: host` and host ranges mapped to one container port are not reported. |
| CD302 | invalid-port | error | A `ports` entry that cannot be parsed (bad number, range or protocol). |
| CD401 | depends-on-undefined-service | error | `depends_on` names an undefined service (unless `required: false`). |
| CD402 | depends-on-without-readiness | info | `depends_on` that only waits for the dependency to start. Advisory: Compose cannot know when a database is ready. |
| CD403 | healthy-condition-without-healthcheck | warning | `condition: service_healthy` on a service without a Compose `healthcheck` (or with `disable: true`). The image may define `HEALTHCHECK`, which an offline tool cannot see, so this is a warning, not an error. |
| CD501 | undefined-volume | error | A named volume used by a service but not declared under top-level `volumes`. |
| CD502 | undefined-network | error | A network joined by a service but not declared under top-level `networks` (`default` is implicit; skipped with `network_mode`). |

## Inactive profiles

Services whose `profiles` are not enabled are ignored by every service rule,
as Docker Compose ignores them. Use `--profile name` (repeatable, `*` for all)
or `COMPOSE_PROFILES`.

## Secrets

Messages, evidence and suggestions contain variable names, service names,
paths and the raw text written in the Compose file, never interpolated or
environment values. This is tested with synthetic secrets in
`internal/rules/vars_test.go`, `internal/cli/cli_test.go`,
`internal/cli/golden_test.go` and `e2e/e2e_test.go`.
| CD601 | restart-policy-missing | info | Advisory (--advisory): a service has no `restart` policy. |
| CD602 | resource-limits-missing | info | Advisory (--advisory): a service sets no `deploy.resources.limits`. |
| CD603 | latest-tag | warning | Advisory (--advisory): an image has the `latest` tag or none. |
