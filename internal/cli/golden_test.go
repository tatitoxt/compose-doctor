package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tatitoxt/compose-doctor/internal/compose"
)

var update = flag.Bool("update", false, "rewrite golden files in testdata/golden")

const (
	fixturesDir = "../../testdata/fixtures"
	goldenDir   = "../../testdata/golden"
)

// fixtureArgs returns the extra CLI arguments stored in <fixture>/args (one per line).
func fixtureArgs(t *testing.T, dir string) []string {
	data, err := os.ReadFile(filepath.Join(dir, "args"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(data))
}

func runFixture(t *testing.T, name, format string) (string, int) {
	dir := filepath.Join(fixturesDir, name)
	args := append([]string{"check", "--format", format}, fixtureArgs(t, dir)...)
	args = append(args, dir)
	var stdout, stderr bytes.Buffer
	// The process environment is not passed: goldens must not depend on the host.
	code := Run(args, &stdout, &stderr, nil)
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %s", stderr.String())
	}
	return stdout.String(), code
}

func TestGoldenFixtures(t *testing.T) {
	entries, err := os.ReadDir(fixturesDir)
	if err != nil {
		t.Fatal(err)
	}
	secrets := syntheticSecrets(t)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			human, code := runFixture(t, name, "human")
			jsonOut, jsonCode := runFixture(t, name, "json")
			if code != jsonCode {
				t.Errorf("exit code differs between formats: human=%d json=%d", code, jsonCode)
			}
			var doc map[string]any
			if err := json.Unmarshal([]byte(jsonOut), &doc); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			for _, s := range secrets {
				if strings.Contains(human, s) || strings.Contains(jsonOut, s) {
					t.Errorf("output leaks the value %q from a fixture env file", s)
				}
			}
			compareGolden(t, filepath.Join(goldenDir, name+".txt"), fmt.Sprintf("exit=%d\n%s", code, human))
			compareGolden(t, filepath.Join(goldenDir, name+".json"), jsonOut)
		})
	}
}

func compareGolden(t *testing.T, path, got string) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run `go test ./internal/cli -update` to create it)", err)
	}
	if got != string(want) {
		t.Errorf("%s mismatch (run `go test ./internal/cli -update` and review the diff)\n--- got\n%s\n--- want\n%s", path, got, want)
	}
}

// syntheticSecrets collects every non-trivial value from fixture env files so
// the golden test can assert that no value is ever printed.
func syntheticSecrets(t *testing.T) []string {
	var values []string
	err := filepath.WalkDir(fixturesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.Contains(d.Name(), ".env") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, v := range compose.ParseDotenv(data) {
			if len(v) >= 6 {
				values = append(values, v)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(values) == 0 {
		t.Fatal("expected synthetic values in fixture env files")
	}
	return values
}
