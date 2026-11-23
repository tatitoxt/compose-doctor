package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The shipped GitHub Action must stay a valid composite action that installs a
// release asset named like the GoReleaser template and runs `check`.
func TestActionManifest(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "action.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatalf("action.yml is not valid YAML: %v", err)
	}
	runs, _ := m["runs"].(map[string]any)
	if runs["using"] != "composite" {
		t.Fatalf("runs.using = %v, want composite", runs["using"])
	}
	steps, _ := runs["steps"].([]any)
	var runsText string
	for _, s := range steps {
		if st, ok := s.(map[string]any); ok {
			if r, ok := st["run"].(string); ok {
				runsText += r + "\n"
			}
		}
	}
	for _, want := range []string{"sha256sum -c", "check", "checksums.txt"} {
		if !strings.Contains(runsText, want) {
			t.Fatalf("action steps missing %q:\n%s", want, runsText)
		}
	}
	inputs, _ := m["inputs"].(map[string]any)
	if _, ok := inputs["args"]; !ok {
		t.Fatal("inputs.args missing")
	}
	if _, ok := inputs["version"]; !ok {
		t.Fatal("inputs.version missing")
	}
}
