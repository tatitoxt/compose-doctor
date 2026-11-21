package rules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tatitoxt/compose-doctor/internal/compose"
)

func loadFiles(t *testing.T, files map[string]string, environ []string, profiles ...string) *compose.Project {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, name)
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p, err := compose.Load(compose.Options{Path: dir, Environ: environ, Profiles: profiles})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func ids(fs []Finding) string {
	var out []string
	for _, f := range fs {
		out = append(out, f.RuleID)
	}
	return strings.Join(out, ",")
}

func TestInterpolatedNamesAreNotPrinted(t *testing.T) {
	const secret = "synthetic-name-secret"
	p := loadFiles(t, map[string]string{"compose.yaml": `services:
  web:
    image: x
    networks: ["${NET}"]
    depends_on: ["${DEP}"]
    ports: ["${P}:80/${PROTO}", "${P}:81/${PROTO}"]
`}, []string{"NET=" + secret, "DEP=" + secret, "P=8080", "PROTO=udp"})
	out := fmt.Sprint(Run(p, nil))
	if strings.Contains(out, secret) || strings.Contains(out, "udp") || strings.Contains(out, "8080") {
		t.Errorf("findings leak resolved values: %s", out)
	}
	if got := ids(Run(p, nil)); !strings.Contains(got, "CD401") || !strings.Contains(got, "CD502") {
		t.Errorf("unexpected findings %s", got)
	}
}

func TestSharedMergeAnchorsStayFast(t *testing.T) {
	var b strings.Builder
	b.WriteString("x0: &a0 {k0: v}\n")
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&b, "x%d: &a%d {<<: [*a%d, *a%d, *a%d], k%d: v}\n", i, i, i-1, i-1, i-1, i)
	}
	b.WriteString("services:\n  s:\n    <<: *a40\n    image: x\n")
	start := time.Now()
	loadFiles(t, map[string]string{"compose.yaml": b.String()}, nil)
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("loading took %v", d)
	}
}

func TestOverridesMergeFieldWise(t *testing.T) {
	p := loadFiles(t, map[string]string{
		"compose.yaml": `services:
  api:
    build: {context: ./api}
    env_file: [{path: ./api.env, required: false}]
    profiles: [dev]
    depends_on: {db: {condition: service_healthy}}
  db:
    image: postgres
    healthcheck: {disable: true}
`,
		"compose.override.yaml": `services:
  api:
    build: {args: {A: "1"}}
    env_file: [./api.env]
    profiles: [debug]
    depends_on: {db: {restart: true}}
  db:
    healthcheck: {interval: 5s}
`,
		"api/Dockerfile": "FROM scratch\n",
	}, nil, "dev")
	api := p.Services["api"]
	if api.Build.Context.Value != "./api" {
		t.Errorf("build context lost: %q", api.Build.Context.Value)
	}
	if len(api.Profiles) != 2 || !p.IsActive(api) {
		t.Errorf("profiles not merged: %v", api.Profiles)
	}
	got := ids(Run(p, nil))
	if !strings.Contains(got, "CD101") || !strings.Contains(got, "CD403") {
		t.Errorf("want required env_file (CD101) and disabled healthcheck (CD403), got %s", got)
	}
}

func TestDotenvValuesAreInterpolated(t *testing.T) {
	p := loadFiles(t, map[string]string{
		".env":         "BASE=80\nPORT=${BASE:-1}80\nLIT='${BASE}'\n",
		"compose.yaml": "services:\n  web:\n    image: x\n    ports: [\"${PORT}:80\", \"8080:81\"]\n",
	}, nil)
	if got := ids(Run(p, nil)); got != "CD301" {
		t.Errorf("want CD301 for 8080 vs 8080, got %q", got)
	}
}

func TestEquivalentPortsInOneServiceAreMerged(t *testing.T) {
	p := loadFiles(t, map[string]string{
		"compose.yaml":          "services:\n  web:\n    image: x\n    ports: [\"${PORT:-8080}:80\"]\n",
		"compose.override.yaml": "services:\n  web:\n    ports: [\"8080:80\"]\n",
	}, nil)
	if got := ids(Run(p, nil)); got != "" {
		t.Errorf("want no findings, got %s", got)
	}
}

func TestResetTagsRemoveResources(t *testing.T) {
	p := loadFiles(t, map[string]string{
		"compose.yaml": `services:
  db:
    image: x
    volumes: [data:/data]
  old:
    restart: always
volumes:
  data: {}
`,
		"compose.override.yaml": "services:\n  old: !reset {}\nvolumes: !reset {}\n",
	}, nil)
	if got := ids(Run(p, nil)); got != "CD501" {
		t.Errorf("want only CD501 (volume reset, service removed), got %q", got)
	}
}

func TestDependencyOverrideRestoresDefaults(t *testing.T) {
	p := loadFiles(t, map[string]string{
		"compose.yaml":          "services:\n  web:\n    image: x\n    depends_on: {db: {condition: service_started, required: false}}\n",
		"compose.override.yaml": "services:\n  web:\n    depends_on:\n      db: !override {condition: service_started}\n",
	}, nil)
	if got := ids(Run(p, nil)); !strings.Contains(got, "CD401") {
		t.Errorf("want CD401 after !override drops required: false, got %q", got)
	}
}

func TestDeepAnchorReusedShallowKeepsFields(t *testing.T) {
	var b strings.Builder
	b.WriteString("base: &b {image: inherited}\nx0: &a0 {<<: *b}\n")
	for i := 1; i <= 31; i++ {
		fmt.Fprintf(&b, "x%d: &a%d {<<: *a%d}\n", i, i, i-1)
	}
	b.WriteString("services:\n  shallow:\n    <<: [*a31, *a0]\n")
	p := loadFiles(t, map[string]string{"compose.yaml": b.String()}, nil)
	if s := p.Services["shallow"]; s == nil || s.Image == nil {
		t.Error("shallow service lost its inherited image")
	}
}
