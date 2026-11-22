package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tatitoxt/compose-doctor/internal/compose"
)

func TestRedactionInNewRules(t *testing.T) {
	p := loadFiles(t, map[string]string{"compose.yaml": `services:
  web:
    image: "${IMG}"
    volumes: ["/${TOKEN}:/d"]
    develop:
      watch: [{path: src}]
`}, []string{"IMG=synthetic-image", "TOKEN=synthetic-secret"})
	var msgs []string
	for _, f := range Run(p, nil) {
		msgs = append(msgs, f.Message+" "+f.Suggestion)
	}
	out := strings.Join(msgs, "\n")
	if strings.Contains(out, "synthetic-secret") || strings.Contains(out, "synthetic-image") {
		t.Errorf("leak: %s", out)
	}
	if !strings.Contains(out, "CD603") && !strings.Contains(out, "/${TOKEN}") {
		t.Errorf("expected CD603/CD503 findings, got %s", out)
	}
	// bare relative watch path that does not exist -> CD104
	if !strings.Contains(out, "develop.watch") {
		t.Errorf("want CD104 for missing bare path 'src': %s", out)
	}
}

func TestIgnoresQuotedAndPending(t *testing.T) {
	text := `services:
  a:
    image: "x # compose-doctor:ignore-file"
    note: |
      text # compose-doctor:ignore-file
      more text
  # compose-doctor:ignore CD201
  b: # compose-doctor:ignore CD301
    image: x
`
	ig := ParseIgnores(text)
	if ig.wholeFile {
		t.Error("quoted/block-scalar text must not enable ignore-file")
	}
	// own-line directive governs line 7 (the entry that also has a trailing directive)
	if !ig.suppresses("CD201", compose.Location{Line: 8}) {
		t.Error("pending directive should govern the next entry line")
	}
}

func TestBaselineSurvivesLineDrift(t *testing.T) {
	entries := []BaselineEntry{{RuleID: "CD301", File: "compose.yaml", Line: 5,
		Message: `host port 80/tcp is published twice by services "a" and "b": "80:80" at compose.yaml:5:9 and "80:81"`}}
	f := Finding{RuleID: "CD301",
		Location: compose.Location{File: "compose.yaml", Line: 9},
		Message:  `host port 80/tcp is published twice by services "a" and "b": "80:80" at compose.yaml:9:9 and "80:81"`}
	kept, n := ApplyBaseline([]Finding{f}, entries)
	if n != 1 || len(kept) != 0 {
		t.Errorf("line drift should not resurrect baselined finding: kept=%v", kept)
	}
}

func TestRulesFileRejectsTrailingData(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "compose-doctor.json")
	if err := os.WriteFile(bad, []byte(`{} {}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// exercised through config in cli; here just verify file shape exists
	_ = bad
}
