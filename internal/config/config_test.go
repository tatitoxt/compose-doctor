package config

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/tatitoxt/compose-doctor/internal/rules"
)

func TestParseCheckDefaults(t *testing.T) {
	c, err := ParseCheck(nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if c.Path != "." || c.Format != "human" || c.Strict || c.FailOn != rules.Warning || len(c.Disabled) != 0 {
		t.Errorf("defaults: %+v", c)
	}
}

func TestParseCheckFlagsAroundPath(t *testing.T) {
	c, err := ParseCheck([]string{"--format", "json", "./proj", "-f", "a.yaml", "-f", "b.yaml", "--strict", "--fail-on=error",
		"--disable", "cd402,CD003", "--profile", "dev", "--ignore-os-env"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if c.Path != "./proj" || c.Format != "json" || !c.Strict || c.FailOn != rules.Error || !c.IgnoreOSEnv {
		t.Errorf("parsed: %+v", c)
	}
	if !reflect.DeepEqual(c.Files, []string{"a.yaml", "b.yaml"}) || !reflect.DeepEqual(c.Profiles, []string{"dev"}) {
		t.Errorf("lists: %+v", c)
	}
	if !c.Disabled["CD402"] || !c.Disabled["CD003"] {
		t.Errorf("disabled: %v", c.Disabled)
	}
}

func TestParseCheckErrors(t *testing.T) {
	for args, want := range map[string]string{
		"--format=xml":    "invalid --format",
		"--fail-on=fatal": "invalid --fail-on",
		"--disable=CD999": "unknown rule",
		"--disable=CD001": "cannot be disabled",
		"a b":             "at most one path",
		"--no-such-flag":  "not defined",
	} {
		_, err := ParseCheck(strings.Fields(args), io.Discard)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v want %q", args, err, want)
		}
	}
	if _, err := ParseCheck([]string{"-h"}, io.Discard); !errors.Is(err, ErrHelp) {
		t.Errorf("-h: %v", err)
	}
}
