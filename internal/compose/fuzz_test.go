package compose

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzInterpolate(f *testing.F) {
	for _, s := range []string{"", "$A", "${A:-x}", "${A:?e}", "${A:+${B:-c}}", "$$", "${", "${A", "${A:-${B", "$1", "${-}"} {
		f.Add(s)
	}
	lookup := func(name string) (string, bool) { return "v", name == "A" }
	f.Fuzz(func(t *testing.T, s string) {
		_, _, _ = Interpolate(s, lookup)
	})
}

// FuzzLoad checks that arbitrary input never panics the loader and that it
// either returns a project or a LoadError.
func FuzzLoad(f *testing.F) {
	for _, s := range []string{
		"services:\n  a:\n    image: x\n",
		"x: &a {b: 1}\nservices:\n  s:\n    <<: *a\n",
		"a: &a [*a]\n",
		"services:\n  s:\n    ports: [\"${P:-80}:80\"]\n    depends_on: {d: {condition: service_healthy}}\n",
		"services: !reset {}\n",
		"\t",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), data, 0o644); err != nil {
			t.Fatal(err)
		}
		p, err := Load(Options{Path: dir})
		if err == nil && p == nil {
			t.Fatal("nil project without error")
		}
		if _, ok := err.(*LoadError); err != nil && !ok {
			t.Fatalf("unexpected error type %T: %v", err, err)
		}
	})
}
