package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(args []string, environ []string) (string, string, int) {
	var out, errOut bytes.Buffer
	code := Run(args, &out, &errOut, environ)
	return out.String(), errOut.String(), code
}

func writeProject(t *testing.T, compose string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestUsageAndVersion(t *testing.T) {
	if _, errOut, code := run(nil, nil); code != ExitUsage || !strings.Contains(errOut, "Usage:") {
		t.Errorf("no args: code=%d stderr=%q", code, errOut)
	}
	if _, _, code := run([]string{"bogus"}, nil); code != ExitUsage {
		t.Errorf("unknown command: code=%d", code)
	}
	if out, _, code := run([]string{"version"}, nil); code != ExitOK || out != "compose-doctor dev\n" {
		t.Errorf("version: code=%d out=%q", code, out)
	}
	if _, errOut, code := run([]string{"check", "--format", "xml"}, nil); code != ExitUsage || !strings.Contains(errOut, "invalid --format") {
		t.Errorf("bad flag: code=%d stderr=%q", code, errOut)
	}
	if _, errOut, code := run([]string{"check", "-h"}, nil); code != ExitOK || !strings.Contains(errOut, "--strict") {
		t.Errorf("check -h: code=%d", code)
	}
}

func TestStrictThresholds(t *testing.T) {
	dir := writeProject(t, "services:\n  web:\n    image: \"nginx:${TAG}\"\n") // one CD201 warning
	cases := []struct {
		args []string
		want int
	}{
		{[]string{"check", dir}, ExitOK},
		{[]string{"check", "--strict", dir}, ExitFindings},
		{[]string{"check", "--strict", "--fail-on", "error", dir}, ExitOK},
		{[]string{"check", "--strict", "--fail-on", "info", dir}, ExitFindings},
		{[]string{"check", "--strict", "--disable", "CD201", dir}, ExitOK},
	}
	for _, c := range cases {
		if _, _, code := run(c.args, nil); code != c.want {
			t.Errorf("%v: exit %d, want %d", c.args, code, c.want)
		}
	}
	if _, _, code := run([]string{"check", "--strict", dir}, []string{"TAG=1.27"}); code != ExitOK {
		t.Errorf("TAG from environment should resolve the warning, exit %d", code)
	}
	if _, _, code := run([]string{"check", "--strict", "--ignore-os-env", dir}, []string{"TAG=1.27"}); code != ExitFindings {
		t.Errorf("--ignore-os-env should ignore TAG, exit %d", code)
	}
}

func TestInvalidInputExitCodeWithStrictAndJSON(t *testing.T) {
	dir := writeProject(t, "services: [\n")
	out, _, code := run([]string{"check", "--format", "json", dir}, nil)
	if code != ExitInvalidInput {
		t.Fatalf("exit %d", code)
	}
	var doc struct {
		Findings []struct {
			RuleID string `json:"rule_id"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || len(doc.Findings) != 1 || doc.Findings[0].RuleID != "CD001" {
		t.Errorf("json: %v %s", err, out)
	}
	if _, _, code := run([]string{"check", "--strict", filepath.Join(dir, "missing.yaml")}, nil); code != ExitInvalidInput {
		t.Errorf("missing file exit %d", code)
	}
}

func TestEnvironmentValuesAreNeverPrinted(t *testing.T) {
	const secret = "synthetic-shell-secret-42"
	dir := writeProject(t, "services:\n  web:\n    image: \"nginx:${TAG}\"\n    environment:\n      TOKEN: ${TOKEN}\n    ports: [\"${PORT}:80\", \"${PORT}:81\"]\n")
	for _, format := range []string{"human", "json"} {
		out, _, _ := run([]string{"check", "--format", format, dir}, []string{"TOKEN=" + secret, "PORT=18080"})
		if strings.Contains(out, secret) || strings.Contains(out, "18080") {
			t.Errorf("%s output leaks an environment value:\n%s", format, out)
		}
		if !strings.Contains(out, "CD301") || !strings.Contains(out, "TAG") {
			t.Errorf("%s output misses expected findings:\n%s", format, out)
		}
	}
}

func TestRulesCommand(t *testing.T) {
	out, _, code := run([]string{"rules"}, nil)
	if code != ExitOK || !strings.Contains(out, "CD301  error    host-port-conflict") {
		t.Errorf("rules: %d %q", code, out)
	}
	out, _, code = run([]string{"rules", "--format", "json"}, nil)
	var list []map[string]any
	if code != ExitOK || json.Unmarshal([]byte(out), &list) != nil || len(list) < 10 {
		t.Errorf("rules json: %d %q", code, out)
	}
	if _, _, code := run([]string{"rules", "--bogus"}, nil); code != ExitUsage {
		t.Errorf("rules bad flag: %d", code)
	}
}

func TestFixRemovesObsoleteVersionKey(t *testing.T) {
	dir := writeProject(t, "version: \"3.8\"\nservices:\n  app:\n    image: myapp\n")
	out, errOut, code := run([]string{"check", "--fix", dir}, nil)
	if code != ExitOK {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
	if !strings.Contains(errOut, "fixed CD003") {
		t.Errorf("expected fix report on stderr, got %q", errOut)
	}
	data, err := os.ReadFile(filepath.Join(dir, "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "version") {
		t.Errorf("version key not removed: %q", data)
	}
	if !strings.Contains(out, "No findings") {
		t.Errorf("expected no remaining findings, got %q", out)
	}
}

func TestFixAddsRestartPolicy(t *testing.T) {
	dir := writeProject(t, "services:\n  app:\n    image: myapp\n    ports:\n      - \"80\"\n  bare:\n")
	out, errOut, code := run([]string{"check", "--advisory", "--fix", dir}, nil)
	if !strings.Contains(errOut, "fixed CD601") {
		t.Fatalf("expected CD601 fix report, got code=%d err=%q out=%q", code, errOut, out)
	}
	data, err := os.ReadFile(filepath.Join(dir, "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "  app:\n    restart: unless-stopped\n") {
		t.Errorf("restart not inserted under app: %q", data)
	}
	if !strings.Contains(string(data), "  bare:\n") {
		t.Errorf("bare service must be left alone: %q", data)
	}
	if strings.Contains(out, `service "app" has no restart policy`) {
		t.Errorf("fixed finding should not remain: %q", out)
	}
	if !strings.Contains(out, `service "bare"`) {
		t.Errorf("bare service's unfixable finding should remain: %q", out)
	}
}

func TestFixLeavesNonFixableFindings(t *testing.T) {
	dir := writeProject(t, "services:\n  app:\n    depends_on:\n      - db\n  db:\n    image: postgres:17\n")
	out, errOut, code := run([]string{"check", "--fix", dir}, nil)
	if code != ExitOK {
		t.Fatalf("code=%d", code)
	}
	if strings.Contains(errOut, "fixed ") {
		t.Errorf("nothing should have been fixed: %q", errOut)
	}
	if !strings.Contains(out, "CD002") || !strings.Contains(out, "CD402") {
		t.Errorf("expected CD002 and CD402 findings to remain, got %q", out)
	}
}
