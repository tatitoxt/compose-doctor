package compose

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func mustLoad(t *testing.T, opts Options) *Project {
	t.Helper()
	p, err := Load(opts)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return p
}

func TestDiscoveryAndOverride(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"compose.yaml":                 "services:\n  web:\n    image: nginx\n    ports: [\"8080:80\"]\n",
		"compose.override.yml":         "services:\n  web:\n    ports: [\"8080:80\", \"9090:90\"]\n",
		"docker-compose.yml":           "services: {ignored: {image: x}}\n",
		"docker-compose.override.yaml": "services: {ignored2: {image: x}}\n",
	})
	p := mustLoad(t, Options{Path: dir})
	if !reflect.DeepEqual(p.Files, []string{"compose.yaml", "compose.override.yml"}) {
		t.Fatalf("files: %v", p.Files)
	}
	if got := p.ServiceNames(); !reflect.DeepEqual(got, []string{"web"}) {
		t.Fatalf("services: %v", got)
	}
	if n := len(p.Services["web"].Ports); n != 2 {
		t.Fatalf("expected 2 merged ports (duplicate removed), got %d", n)
	}
}

func TestExplicitFilesAndPathAsFile(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"base.yaml": "services:\n  api:\n    image: api\n    depends_on: [db]\n",
		"prod.yaml": "services:\n  api:\n    image: api:prod\n    depends_on:\n      db:\n        condition: service_healthy\n",
	})
	p := mustLoad(t, Options{Path: dir, Files: []string{"base.yaml", "prod.yaml"}})
	api := p.Services["api"]
	if api.Image.Value != "api:prod" {
		t.Errorf("image override: %q", api.Image.Value)
	}
	if len(api.DependsOn) != 1 || api.DependsOn[0].Condition != "service_healthy" || api.DependsOn[0].Loc.File != "prod.yaml" {
		t.Errorf("depends_on merge: %+v", api.DependsOn)
	}
	p = mustLoad(t, Options{Path: filepath.Join(dir, "base.yaml")})
	if !reflect.DeepEqual(p.Files, []string{"base.yaml"}) {
		t.Errorf("path as file: %v", p.Files)
	}
}

func TestLoadErrors(t *testing.T) {
	dir := writeTree(t, map[string]string{"README.md": "x"})
	var le *LoadError
	if _, err := Load(Options{Path: dir}); !errors.As(err, &le) || !strings.Contains(le.Msg, "no Compose file found") {
		t.Fatalf("expected no-compose error, got %v", err)
	}
	if _, err := Load(Options{Path: filepath.Join(dir, "nope")}); !errors.As(err, &le) {
		t.Fatalf("expected path error, got %v", err)
	}
	if _, err := Load(Options{Path: dir, Files: []string{"missing.yaml"}}); !errors.As(err, &le) || !strings.Contains(le.Msg, "no such file") {
		t.Fatalf("expected missing -f error, got %v", err)
	}
	d2 := writeTree(t, map[string]string{"compose.yaml": "services: [a]\n"})
	if _, err := Load(Options{Path: d2}); !errors.As(err, &le) || !strings.Contains(le.Msg, "must be a mapping") {
		t.Fatalf("expected services mapping error, got %v", err)
	}
	if _, err := Load(Options{Path: d2, EnvFile: filepath.Join(d2, "missing.env")}); !errors.As(err, &le) {
		t.Fatalf("expected env file error, got %v", err)
	}
}

func TestInterpolationSourcesAndPrecedence(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"compose.yaml": "services:\n  web:\n    image: \"nginx:${TAG}\"\n    ports: [\"${PORT}:80\"]\n    environment:\n      A: $MISSING\n      B: $${ESCAPED}\n",
		".env":         "TAG=from-dotenv\nPORT=8080\n",
		".env.example": "TAG=\nPORT=\nMISSING=\n",
	})
	p := mustLoad(t, Options{Path: dir, Environ: []string{"TAG=from-shell"}})
	if got := p.Services["web"].Image.Value; got != "nginx:from-shell" {
		t.Errorf("shell env must win over .env: %q", got)
	}
	if v := p.Services["web"].Ports[0].Short; v.Value != "8080:80" || !v.Interpolated || v.Unresolved {
		t.Errorf("port value: %+v", v)
	}
	var problems []string
	for _, u := range p.VarUses {
		if u.Ref.Problem {
			problems = append(problems, u.Ref.Name+"@"+u.Path)
		}
	}
	if !reflect.DeepEqual(problems, []string{"MISSING@services.web.environment.A"}) {
		t.Errorf("problems: %v", problems)
	}
	if !p.InEnvExample("MISSING") || p.DotenvFile != ".env" {
		t.Errorf("env example / dotenv not detected")
	}
}

func TestAnchorsMergeKeysAndResetTags(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"compose.yaml": `x-common: &common
  env_file: [./common.env]
  healthcheck:
    test: ["CMD", "true"]
services:
  a:
    <<: *common
    image: a
    ports: ["1000:1000"]
  b:
    <<: *common
    image: b
`,
		"compose.override.yaml": "services:\n  a:\n    ports: !reset []\n  b:\n    env_file: !override [./b.env]\n",
	})
	p := mustLoad(t, Options{Path: dir})
	a, b := p.Services["a"], p.Services["b"]
	if a.Healthcheck == nil || len(a.EnvFiles) != 1 || a.EnvFiles[0].Path.Value != "./common.env" {
		t.Errorf("anchor merge failed for a: %+v", a)
	}
	if len(a.Ports) != 0 {
		t.Errorf("!reset should clear ports: %+v", a.Ports)
	}
	if len(b.EnvFiles) != 1 || b.EnvFiles[0].Path.Value != "./b.env" {
		t.Errorf("!override should replace env_file: %+v", b.EnvFiles)
	}
}

func TestProfiles(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"compose.yaml": "services:\n  web: {image: w}\n  debug: {image: d, profiles: [debug]}\n",
	})
	names := func(p *Project) []string {
		var out []string
		for _, s := range p.ActiveServices() {
			out = append(out, s.Name)
		}
		return out
	}
	if got := names(mustLoad(t, Options{Path: dir})); !reflect.DeepEqual(got, []string{"web"}) {
		t.Errorf("default profiles: %v", got)
	}
	if got := names(mustLoad(t, Options{Path: dir, Profiles: []string{"debug"}})); len(got) != 2 {
		t.Errorf("--profile debug: %v", got)
	}
	if got := names(mustLoad(t, Options{Path: dir, Environ: []string{"COMPOSE_PROFILES=other,debug"}})); len(got) != 2 {
		t.Errorf("COMPOSE_PROFILES: %v", got)
	}
}

func TestDuplicateKeysAndVersion(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"compose.yaml": "version: \"3.8\"\nservices:\n  web:\n    image: a\n    image: b\n",
	})
	p := mustLoad(t, Options{Path: dir})
	if len(p.DupKeys) != 1 || p.DupKeys[0].Path != "services.web.image" || p.DupKeys[0].Loc.Line != 5 || p.DupKeys[0].First.Line != 4 {
		t.Errorf("dup keys: %+v", p.DupKeys)
	}
	if len(p.VersionKeys) != 1 || p.VersionKeys[0].Line != 1 {
		t.Errorf("version: %+v", p.VersionKeys)
	}
}

func TestSplitShortVolume(t *testing.T) {
	cases := []struct{ in, src, target, typ string }{
		{"/data", "", "/data", "volume"},
		{"db:/var/lib/db", "db", "/var/lib/db", "volume"},
		{"./data:/data:ro", "./data", "/data", "bind"},
		{"~/x:/x", "~/x", "/x", "bind"},
		{`C:\data:/data`, `C:\data`, "/data", "bind"},
		{"/abs/path:/in", "/abs/path", "/in", "bind"},
	}
	for _, c := range cases {
		src, target := SplitShortVolume(c.in)
		if src != c.src || target != c.target || shortVolumeType(src) != c.typ {
			t.Errorf("%q: got (%q,%q,%s)", c.in, src, target, shortVolumeType(src))
		}
	}
}
