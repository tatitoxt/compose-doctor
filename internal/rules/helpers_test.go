package rules

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tatitoxt/compose-doctor/internal/compose"
)

type ruleCase struct {
	name    string
	files   map[string]string
	environ []string
	opts    compose.Options
	// want lists "RULE@file:line" for findings of the rules under test.
	want []string
}

func load(t *testing.T, files map[string]string, opts compose.Options) *compose.Project {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	opts.Path = dir
	p, err := compose.Load(opts)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return p
}

func runCases(t *testing.T, ruleIDs []string, cases []ruleCase) {
	t.Helper()
	wanted := map[string]bool{}
	for _, id := range ruleIDs {
		wanted[id] = true
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := tc.opts
			opts.Environ = tc.environ
			p := load(t, tc.files, opts)
			var got []string
			for _, f := range Run(p, nil) {
				if wanted[f.RuleID] {
					got = append(got, f.RuleID+"@"+f.Location.String())
					if f.Message == "" || f.Suggestion == "" {
						t.Errorf("%s: empty message or suggestion", f.RuleID)
					}
				}
			}
			for i, w := range tc.want {
				// Allow omitting the column in expectations.
				if i < len(got) && strings.Count(w, ":") == 1 && strings.HasPrefix(got[i], w+":") {
					got[i] = w
				}
			}
			if !reflect.DeepEqual(got, tc.want) && !(len(got) == 0 && len(tc.want) == 0) {
				t.Errorf("findings:\n got  %v\n want %v", got, tc.want)
			}
		})
	}
}

func findOne(t *testing.T, files map[string]string, environ []string, id string) Finding {
	t.Helper()
	p := load(t, files, compose.Options{Environ: environ})
	for _, f := range Run(p, nil) {
		if f.RuleID == id {
			return f
		}
	}
	t.Fatalf("no %s finding", id)
	return Finding{}
}
