GO ?= go
STATICCHECK_VERSION := v0.7.0
GOVULNCHECK_VERSION := v1.8.0

.PHONY: build test unit e2e race lint vuln fuzz golden demo ci

build:
	$(GO) build -trimpath -o bin/compose-doctor ./cmd/compose-doctor

## test: unit, integration (golden) and E2E tests
test:
	$(GO) test ./...

unit:
	$(GO) test ./internal/...

e2e:
	$(GO) test ./e2e/...

race:
	$(GO) test -race ./...

lint:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && echo "run gofmt -w ." && exit 1)
	$(GO) vet ./...
	$(GO) run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) ./...

vuln:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

fuzz:
	$(GO) test ./internal/compose -run '^$$' -fuzz FuzzInterpolate -fuzztime 30s
	$(GO) test ./internal/compose -run '^$$' -fuzz FuzzLoad -fuzztime 30s

## golden: regenerate testdata/golden after an intended output change
golden:
	$(GO) test ./internal/cli -run TestGoldenFixtures -update

demo: build
	-./bin/compose-doctor check testdata/fixtures/undefined-variables
	-./bin/compose-doctor check --strict testdata/fixtures/port-conflicts; echo "exit code: $$?"

ci: lint test race vuln
