package config

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tatitoxt/compose-doctor/internal/rules"
)

func writeRulesFile(t *testing.T, dir, body string) string {
	t.Helper()
	p := filepath.Join(dir, RulesFileName)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRulesFileAutoDetectedAndApplied(t *testing.T) {
	dir := t.TempDir()
	writeRulesFile(t, dir, `{"severity": {"cd102": "info", "CD201": "off"}}`)
	c, err := ParseCheck([]string{dir}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ApplyRulesFile(); err != nil {
		t.Fatal(err)
	}
	if !c.Disabled["CD201"] || c.Severities["CD102"] != rules.Info {
		t.Fatalf("disabled=%v severities=%v", c.Disabled, c.Severities)
	}
}

func TestRulesFileExplicitConfigAndCliDisableUnion(t *testing.T) {
	dir := t.TempDir()
	cfg := writeRulesFile(t, dir, `{"severity": {"CD201": "off", "CD102": "error"}}`)
	c, err := ParseCheck([]string{"--config", cfg, "--disable", "CD301", dir}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ApplyRulesFile(); err != nil {
		t.Fatal(err)
	}
	if !c.Disabled["CD201"] || !c.Disabled["CD301"] || c.Severities["CD102"] != rules.Error {
		t.Fatalf("disabled=%v severities=%v", c.Disabled, c.Severities)
	}
}

func TestRulesFileMissingDirMeansNoConfig(t *testing.T) {
	dir := t.TempDir()
	c, err := ParseCheck([]string{dir}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ApplyRulesFile(); err != nil {
		t.Fatal(err)
	}
	if len(c.Severities) != 0 {
		t.Fatalf("severities=%v", c.Severities)
	}
}

func TestRulesFileExplicitMissingIsError(t *testing.T) {
	dir := t.TempDir()
	c, err := ParseCheck([]string{"--config", filepath.Join(dir, "nope.json"), dir}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ApplyRulesFile(); err == nil || !strings.Contains(err.Error(), "cannot read") {
		t.Fatalf("err=%v", err)
	}
}

func TestRulesFileRejectsUnknownAndCD001AndBadSeverity(t *testing.T) {
	dir := t.TempDir()
	for body, want := range map[string]string{
		`{"severity": {"CD999": "info"}}`: "unknown rule",
		`{"severity": {"CD001": "off"}}`:  "CD001 cannot be",
		`{"severity": {"CD102": "loud"}}`: "unknown severity",
		`{"unknown": 1}`:                  "unknown field",
		`{bad json`:                       "invalid rules file",
	} {
		writeRulesFile(t, dir, body)
		c, err := ParseCheck([]string{dir}, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		err = c.ApplyRulesFile()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: err=%v want %q", body, err, want)
		}
	}
}
