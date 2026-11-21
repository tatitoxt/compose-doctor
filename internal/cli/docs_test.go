package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/tatitoxt/compose-doctor/internal/rules"
)

// TestRulesAreDocumented keeps docs/rules.md and the README rule table in
// sync with the rule registry.
func TestRulesAreDocumented(t *testing.T) {
	for _, path := range []string{"../../docs/rules.md", "../../README.md"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		doc := string(data)
		for _, r := range rules.All() {
			row := "| " + r.ID + " | " + r.Name + " | " + r.Severity.String() + " |"
			if !strings.Contains(doc, row) {
				t.Errorf("%s: missing or outdated row starting with %q", path, row)
			}
		}
	}
}
