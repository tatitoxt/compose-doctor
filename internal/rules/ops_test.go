package rules

import (
	"strings"
	"testing"

	"github.com/tatitoxt/compose-doctor/internal/compose"
)

func advisoryIDs(t *testing.T, files map[string]string) string {
	t.Helper()
	p := load(t, files, compose.Options{})
	got := ids(Run(p, nil, true))
	return got
}

func TestAdvisoryRulesAreOptIn(t *testing.T) {
	p := load(t, map[string]string{
		"compose.yaml": "services:\n  web:\n    image: nginx:1.25\n",
	}, compose.Options{})
	if got := ids(Run(p, nil)); got != "" {
		t.Fatalf("advisory rules ran without opt-in: %s", got)
	}
	got := advisoryIDs(t, map[string]string{
		"compose.yaml": "services:\n  web:\n    image: nginx:1.25\n",
	})
	for _, want := range []string{"CD601", "CD602"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %s in %s", want, got)
		}
	}
	if strings.Contains(got, "CD603") {
		t.Fatalf("pinned tag flagged: %s", got)
	}
}

func TestAdvisoryRulesSatisfied(t *testing.T) {
	got := advisoryIDs(t, map[string]string{
		"compose.yaml": "services:\n  web:\n    image: nginx:1.25\n    restart: unless-stopped\n    deploy:\n      resources:\n        limits:\n          cpus: '0.5'\n          memory: 256M\n",
	})
	if strings.Contains(got, "CD601") || strings.Contains(got, "CD602") {
		t.Fatalf("satisfied ops config flagged: %s", got)
	}
}

func TestLatestTagDetection(t *testing.T) {
	for name, img := range map[string]string{
		"no tag":          "nginx",
		"explicit latest": "nginx:latest",
		"registry no tag": "ghcr.io/acme/app",
	} {
		got := advisoryIDs(t, map[string]string{
			"compose.yaml": "services:\n  web:\n    image: " + img + "\n    restart: unless-stopped\n",
		})
		if !strings.Contains(got, "CD603") {
			t.Fatalf("%s: want CD603 in %s", name, got)
		}
	}
	got := advisoryIDs(t, map[string]string{
		"compose.yaml": "services:\n  web:\n    image: ghcr.io/acme/app:v2.1@sha256:abc\n    restart: unless-stopped\n",
	})
	if strings.Contains(got, "CD603") {
		t.Fatalf("pinned digest flagged: %s", got)
	}
}
