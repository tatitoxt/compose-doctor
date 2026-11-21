// Package e2e builds the real compose-doctor binary and runs it as a user would.
package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "compose-doctor-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "compose-doctor")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "../cmd/compose-doctor")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "building compose-doctor:", err)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type result struct {
	stdout, stderr string
	code           int
}

// cli runs the binary in dir with a minimal, host-independent environment.
func cli(t *testing.T, dir string, env []string, args ...string) result {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH")}, env...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("running %v: %v", args, err)
	}
	return result{out.String(), errOut.String(), code}
}

// copyFixture copies testdata/fixtures/<name> into a fresh temp directory.
func copyFixture(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join("..", "testdata", "fixtures", name)
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

// snapshot records path, mode, size, mtime and content hash of every entry.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	state := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		entry := fmt.Sprintf("%v %d %d", info.Mode(), info.Size(), info.ModTime().UnixNano())
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			entry += " " + hex.EncodeToString(sum[:])
		}
		state[path] = entry
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

type jsonReport struct {
	SchemaVersion int      `json:"schema_version"`
	Files         []string `json:"files"`
	Findings      []struct {
		RuleID   string `json:"rule_id"`
		Severity string `json:"severity"`
		Location struct {
			File string `json:"file"`
			Line int    `json:"line"`
		} `json:"location"`
	} `json:"findings"`
	Summary struct {
		Total int `json:"total"`
	} `json:"summary"`
}

func decode(t *testing.T, out string) jsonReport {
	t.Helper()
	var r jsonReport
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, out)
	}
	if r.SchemaVersion != 1 || r.Summary.Total != len(r.Findings) {
		t.Fatalf("unexpected report header: %+v", r)
	}
	return r
}

func ruleIDs(r jsonReport) map[string]int {
	ids := map[string]int{}
	for _, f := range r.Findings {
		ids[f.RuleID]++
	}
	return ids
}

func TestValidProjectFromWorkingDirectory(t *testing.T) {
	dir := copyFixture(t, "valid-complex")
	res := cli(t, dir, nil, "check", "--strict", "--fail-on", "info")
	if res.code != 0 || res.stdout != "Checked compose.yaml, compose.override.yaml\nNo findings.\n" || res.stderr != "" {
		t.Fatalf("got %+v", res)
	}
}

func TestAcceptanceScenarios(t *testing.T) {
	cases := []struct {
		fixture  string
		args     []string
		wantCode int
		wantIDs  map[string]int
	}{
		{"undefined-variables", nil, 0, map[string]int{"CD201": 3, "CD202": 1, "CD203": 1}},
		{"undefined-variables", []string{"--strict"}, 1, nil},
		{"missing-files", []string{"--strict", "--fail-on", "error"}, 1, map[string]int{"CD101": 2, "CD102": 2, "CD103": 2}},
		{"port-conflicts", nil, 0, map[string]int{"CD301": 4}},
		{"depends-on", nil, 0, map[string]int{"CD401": 1, "CD402": 3, "CD403": 1}},
		{"depends-on", []string{"--strict", "--disable", "CD401,CD403"}, 0, map[string]int{"CD402": 3}},
		{"references", nil, 0, map[string]int{"CD501": 1, "CD502": 1}},
		{"structure", nil, 0, map[string]int{"CD002": 1, "CD003": 1, "CD004": 1}},
		{"invalid-yaml", nil, 3, map[string]int{"CD001": 1}},
		{"invalid-yaml", []string{"--strict"}, 3, map[string]int{"CD001": 1}},
		{"no-compose-file", nil, 3, map[string]int{"CD001": 1}},
		{"multi-file", []string{"-f", "base.yaml", "-f", "prod.yaml"}, 0, map[string]int{"CD301": 1, "CD403": 1}},
		{"multi-file", []string{"-f", "base.yaml"}, 0, map[string]int{}},
		{"profiles", nil, 0, map[string]int{}},
		{"profiles", []string{"--profile", "debug"}, 0, map[string]int{"CD101": 1, "CD301": 1}},
	}
	for _, c := range cases {
		t.Run(c.fixture+" "+strings.Join(c.args, " "), func(t *testing.T) {
			dir := copyFixture(t, c.fixture)
			before := snapshot(t, dir)
			args := append([]string{"check", "--format", "json"}, c.args...)
			res := cli(t, dir, nil, args...)
			if res.code != c.wantCode {
				t.Errorf("exit code %d, want %d\nstdout: %s\nstderr: %s", res.code, c.wantCode, res.stdout, res.stderr)
			}
			r := decode(t, res.stdout)
			if c.wantIDs != nil && fmt.Sprint(ruleIDs(r)) != fmt.Sprint(c.wantIDs) {
				t.Errorf("rule IDs %v, want %v", ruleIDs(r), c.wantIDs)
			}
			for _, f := range r.Findings {
				if f.Location.File == "" {
					t.Errorf("finding %s has no location", f.RuleID)
				}
			}
			if after := snapshot(t, dir); fmt.Sprint(before) != fmt.Sprint(after) {
				t.Errorf("project directory was modified:\nbefore %v\nafter  %v", before, after)
			}
		})
	}
}

func TestShellEnvironmentResolvesVariablesWithoutPrintingThem(t *testing.T) {
	dir := copyFixture(t, "undefined-variables")
	const secret = "synthetic-e2e-password"
	env := []string{"PG_TAG=16", "DB_PASSWORD=" + secret, "DB_NAME=app", "API_TAG=v1"}
	res := cli(t, dir, env, "check", "--format", "json")
	r := decode(t, res.stdout)
	if ids := ruleIDs(r); fmt.Sprint(ids) != fmt.Sprint(map[string]int{"CD203": 1}) {
		t.Errorf("rule IDs %v, want only CD203", ids)
	}
	for _, out := range []string{res.stdout, cli(t, dir, env, "check").stdout} {
		if strings.Contains(out, secret) {
			t.Errorf("output leaks a variable value:\n%s", out)
		}
	}
}

func TestHumanOutputNamesFileLineRuleAndHint(t *testing.T) {
	dir := copyFixture(t, "missing-files")
	res := cli(t, dir, nil, "check")
	for _, want := range []string{
		`compose.yaml:10:15  error    CD101  service "web": env_file "./web.env" not found`,
		`copy "./web.env.example" to "./web.env"`,
		"6 findings (4 error, 2 warning, 0 info)",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("human output lacks %q:\n%s", want, res.stdout)
		}
	}
}

func TestSARIFOutputMatchesJSONFindings(t *testing.T) {
	dir := copyFixture(t, "missing-files")
	res := cli(t, dir, nil, "check", "--format", "sarif")
	if res.code != 0 {
		t.Fatalf("exit %d\nstderr: %s", res.code, res.stderr)
	}
	var log struct {
		Version string `json:"version"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Rules []struct {
						ID string `json:"id"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID    string `json:"ruleId"`
				Level     string `json:"level"`
				Locations []struct {
					PhysicalLocation struct {
						ArtifactLocation struct {
							URI string `json:"uri"`
						} `json:"artifactLocation"`
						Region struct {
							StartLine int `json:"startLine"`
						} `json:"region"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &log); err != nil {
		t.Fatalf("sarif output is not JSON: %v\n%s", err, res.stdout)
	}
	if log.Version != "2.1.0" || len(log.Runs) != 1 || len(log.Runs[0].Tool.Driver.Rules) == 0 {
		t.Fatalf("bad SARIF envelope: %+v", log)
	}
	catalog := map[string]bool{}
	for _, r := range log.Runs[0].Tool.Driver.Rules {
		catalog[r.ID] = true
	}
	if len(log.Runs[0].Results) != 6 {
		t.Fatalf("results %d, want 6", len(log.Runs[0].Results))
	}
	for _, r := range log.Runs[0].Results {
		if !catalog[r.RuleID] {
			t.Errorf("result %s is not in the driver rule catalog", r.RuleID)
		}
		switch r.Level {
		case "error", "warning", "note":
		default:
			t.Errorf("bad SARIF level %q", r.Level)
		}
		if len(r.Locations) != 1 || r.Locations[0].PhysicalLocation.ArtifactLocation.URI == "" {
			t.Errorf("result %s lacks a physical location", r.RuleID)
		}
		if r.Locations[0].PhysicalLocation.Region.StartLine <= 0 {
			t.Errorf("result %s lacks a start line", r.RuleID)
		}
	}
	// Strict mode honours SARIF output too.
	if res := cli(t, dir, nil, "check", "--format", "sarif", "--strict"); res.code != 1 {
		t.Errorf("strict sarif: exit %d, want 1", res.code)
	}
}

func TestUsageErrors(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{}, {"frobnicate"}, {"check", "--format", "yaml"}, {"check", "--fail-on", "fatal"}, {"check", "a", "b"}} {
		if res := cli(t, dir, nil, args...); res.code != 2 || res.stderr == "" {
			t.Errorf("%v: exit %d stderr %q", args, res.code, res.stderr)
		}
	}
	if res := cli(t, dir, nil, "version"); res.code != 0 || !strings.HasPrefix(res.stdout, "compose-doctor ") {
		t.Errorf("version: %+v", res)
	}
	if res := cli(t, dir, nil, "rules", "--format", "json"); res.code != 0 || !strings.Contains(res.stdout, `"id": "CD001"`) {
		t.Errorf("rules: %+v", res)
	}
}

func TestInlineIgnoreDirectives(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) {
		if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write(`services:
  api:
    image: example/api
    ports:
      - "8080:8080"
  web:
    image: nginx:1.27
    ports:
      - "8080:80"  # compose-doctor:ignore CD301
  db:
    image: example/db
    ports:
      - "9090:9090"
  cache:
    image: example/cache
    ports:
      - "9090:6379"
`)
	res := cli(t, dir, nil, "check", "--format", "json")
	r := decode(t, res.stdout)
	if ids := ruleIDs(r); ids["CD301"] != 1 || len(ids) != 1 {
		t.Fatalf("rule IDs %v, want one CD301", ids)
	}
	if !strings.Contains(res.stderr, "1 finding(s) suppressed") {
		t.Errorf("stderr lacks suppression note: %q", res.stderr)
	}

	write(`# compose-doctor:ignore-file
services:
  api:
    image: example/api
    ports:
      - "8080:8080"
  web:
    image: nginx:1.27
    ports:
      - "8080:80"
`)
	res = cli(t, dir, nil, "check", "--format", "json")
	if r := decode(t, res.stdout); len(r.Findings) != 0 {
		t.Fatalf("ignore-file left findings: %v", ruleIDs(r))
	}
}
