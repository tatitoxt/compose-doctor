package compose

import (
	"reflect"
	"testing"
)

func mapLookup(m map[string]string) Lookup {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestInterpolate(t *testing.T) {
	env := mapLookup(map[string]string{"SET": "value", "EMPTY": "", "PORT": "8080"})
	tests := []struct {
		in       string
		want     string
		problems []string
	}{
		{"plain text", "plain text", nil},
		{"$SET", "value", nil},
		{"${SET}", "value", nil},
		{"${UNSET}", "", []string{"UNSET"}},
		{"$UNSET-x", "-x", []string{"UNSET"}},
		{"${UNSET:-fallback}", "fallback", nil},
		{"${EMPTY:-fallback}", "fallback", nil},
		{"${EMPTY-fallback}", "", nil},
		{"${UNSET-fallback}", "fallback", nil},
		{"${UNSET:?must be set}", "", []string{"UNSET"}},
		{"${EMPTY:?must be set}", "", []string{"EMPTY"}},
		{"${EMPTY?must be set}", "", nil},
		{"${SET:+alt}", "alt", nil},
		{"${EMPTY:+alt}", "", nil},
		{"${EMPTY+alt}", "alt", nil},
		{"${UNSET+alt}", "", nil},
		{"$${NOT_A_VAR} $$HOME", "${NOT_A_VAR} $HOME", nil},
		{"${UNSET:-${PORT}}:80", "8080:80", nil},
		{"${UNSET:-${ALSO_UNSET}}", "", []string{"ALSO_UNSET"}},
		{"${SET:-${ALSO_UNSET}}", "value", nil},
		{"cost: 5$", "cost: 5$", nil},
		{"$1 and $ alone", "$1 and $ alone", nil},
		{"${PORT}${PORT}", "80808080", nil},
	}
	for _, tt := range tests {
		got, refs, err := Interpolate(tt.in, env)
		if err != nil {
			t.Fatalf("%q: unexpected error %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("%q: got %q want %q", tt.in, got, tt.want)
		}
		var problems []string
		for _, r := range refs {
			if r.Problem {
				problems = append(problems, r.Name)
			}
		}
		if !reflect.DeepEqual(problems, tt.problems) {
			t.Errorf("%q: problems %v want %v", tt.in, problems, tt.problems)
		}
	}
}

func TestInterpolateRequiredFlag(t *testing.T) {
	_, refs, err := Interpolate("${DB_PASSWORD:?set it}", mapLookup(nil))
	if err != nil || len(refs) != 1 || !refs[0].Required() || !refs[0].Problem {
		t.Fatalf("unexpected refs %+v err %v", refs, err)
	}
}

func TestInterpolateErrors(t *testing.T) {
	for _, in := range []string{"${", "${UNCLOSED", "${}", "${1BAD}", "${A!b}", "${A:-${B}"} {
		if _, _, err := Interpolate(in, mapLookup(nil)); err == nil {
			t.Errorf("%q: expected error", in)
		}
	}
}

func TestHasInterpolation(t *testing.T) {
	cases := map[string]bool{"8080:80": false, "${P}:80": true, "$P": true, "$$P": false, "5$": false, "$1": false}
	for in, want := range cases {
		if got := HasInterpolation(in); got != want {
			t.Errorf("%q: got %v want %v", in, got, want)
		}
	}
}
