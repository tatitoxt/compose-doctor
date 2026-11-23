package fix

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tatitoxt/compose-doctor/internal/compose"
	"github.com/tatitoxt/compose-doctor/internal/rules"
)

func mkFinding(id, file string, line int) rules.Finding {
	return rules.Finding{RuleID: id, Location: compose.Location{File: file, Line: line}}
}

func TestSymlinkEscapeRefused(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(dir, "outside.yaml")
	if err := os.WriteFile(outside, []byte("version: '3'\nservices: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(dir, "proj")
	os.MkdirAll(proj, 0o755)
	link := filepath.Join(proj, "compose.yaml")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlinks unsupported")
	}
	applied, remaining, err := Apply(proj, []rules.Finding{mkFinding("CD003", "compose.yaml", 1)})
	if err != nil || len(applied) != 0 || len(remaining) != 1 {
		t.Fatalf("want refusal, got applied=%v rem=%v err=%v", applied, remaining, err)
	}
	if data, _ := os.ReadFile(outside); strings.Contains(string(data), "services") && !strings.Contains(string(data), "version") {
		t.Error("external file was modified")
	}
}

func TestVersionAnchorNotRemoved(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("version: &v '3'\nservices:\n  x: {image: a}\nx: {ref: *v}\n"), 0o644)
	applied, remaining, _ := Apply(dir, []rules.Finding{mkFinding("CD003", "compose.yaml", 1)})
	if len(applied) != 0 || len(remaining) != 1 {
		t.Errorf("anchored version must not be removed: %v %v", applied, remaining)
	}
}

func TestRestartFixEdgeCases(t *testing.T) {
	dir := t.TempDir()
	body := "services:\n  a:\n    image: x\n  longer:\n    # comment\n    image: y\n  has:\n    restart: !reset null\n    image: z\n  flow: {image: q}\n"
	os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(body), 0o644)
	fs := []rules.Finding{
		mkFinding("CD601", "compose.yaml", 2),  // 1-char name a
		mkFinding("CD601", "compose.yaml", 4),  // comment then image
		mkFinding("CD601", "compose.yaml", 7),  // has restart already
		mkFinding("CD601", "compose.yaml", 10), // flow mapping
	}
	applied, remaining, err := Apply(dir, fs)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 2 || len(remaining) != 2 {
		t.Fatalf("want 2 fixed 2 remaining, got %v %v", applied, remaining)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "compose.yaml"))
	out := string(data)
	if strings.Count(out, "restart: unless-stopped") != 2 {
		t.Errorf("bad insert: %s", out)
	}
}
