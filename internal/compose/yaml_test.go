package compose

import (
	"errors"
	"strings"
	"testing"
)

func TestParseYAMLErrors(t *testing.T) {
	tests := []struct {
		name, in, msg string
		line          int
	}{
		{"empty", "  \n", "file is empty", 0},
		{"comment only", "# nothing\n", "file is empty", 0},
		// yaml.v3 reports the line where the enclosing block starts.
		{"bad indentation", "services:\n  web:\n    image: nginx\n   ports: [80]\n", "did not find expected key", 1},
		{"tab", "services:\n\tweb: {}\n", "invalid YAML", 2},
		{"not a mapping", "- a\n- b\n", "top-level value must be a mapping", 1},
		{"unclosed flow", "services: {web: {image: x}\n", "invalid YAML", 1},
		{"no line info", "a: b: c\n", "mapping values are not allowed", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseYAML("compose.yaml", []byte(tt.in))
			var le *LoadError
			if !errors.As(err, &le) {
				t.Fatalf("expected LoadError, got %v", err)
			}
			if !strings.Contains(le.Msg, tt.msg) {
				t.Errorf("msg %q does not contain %q", le.Msg, tt.msg)
			}
			if le.Loc.Line != tt.line {
				t.Errorf("line %d want %d (%s)", le.Loc.Line, tt.line, le.Msg)
			}
		})
	}
}

func TestParseYAMLDepthLimit(t *testing.T) {
	in := "a: " + strings.Repeat("[", MaxDepth+5) + strings.Repeat("]", MaxDepth+5) + "\n"
	_, err := parseYAML("compose.yaml", []byte(in))
	if err == nil || !strings.Contains(err.Error(), "nesting deeper") {
		t.Fatalf("expected depth error, got %v", err)
	}
}

func TestMergeKeys(t *testing.T) {
	in := `
x-a: &a {image: a, restart: always}
x-b: &b {image: b, user: root}
one:
  <<: [*a, *b]
  user: app
two:
  <<: *b
`
	root, err := parseYAML("f", []byte(in))
	if err != nil {
		t.Fatal(err)
	}
	_, one := lookup(root, "one")
	got := map[string]string{}
	for _, p := range pairs(one) {
		got[p.Key.Value] = resolve(p.Val).Value
	}
	want := map[string]string{"image": "a", "restart": "always", "user": "app"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("one.%s = %q want %q", k, got[k], v)
		}
	}
	_, two := lookup(root, "two")
	if _, img := lookup(two, "image"); img == nil || img.Value != "b" {
		t.Errorf("two.image not merged: %+v", img)
	}
}
