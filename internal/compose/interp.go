package compose

import (
	"fmt"
	"strings"
)

// Lookup returns the value of a variable and whether it is set.
type Lookup func(name string) (string, bool)

// VarRef is one variable reference evaluated while interpolating a string.
type VarRef struct {
	Name string
	// Op is the modifier used: "", ":-", "-", ":?", "?", ":+" or "+".
	Op string
	// Set reports whether the variable was set (possibly empty).
	Set bool
	// Problem is true when the reference resolves to nothing useful: a plain
	// reference to an unset variable, or a required (?) reference whose
	// condition is not met.
	Problem bool
}

// Required reports whether the reference uses the ":?" or "?" modifier.
func (r VarRef) Required() bool { return r.Op == ":?" || r.Op == "?" }

// InterpError describes malformed interpolation syntax.
type InterpError struct {
	Offset int
	Msg    string
}

func (e *InterpError) Error() string { return fmt.Sprintf("offset %d: %s", e.Offset, e.Msg) }

// HasInterpolation reports whether s contains a "$" that is not part of a "$$" escape.
func HasInterpolation(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '$' {
			continue
		}
		if i+1 < len(s) && s[i+1] == '$' {
			i++
			continue
		}
		if i+1 < len(s) && (s[i+1] == '{' || isNameStart(s[i+1])) {
			return true
		}
	}
	return false
}

// Interpolate expands Compose-style variable references in s.
// Default/alternate words are expanded lazily, so references inside an
// unused default are not reported.
func Interpolate(s string, lookup Lookup) (string, []VarRef, error) {
	var b strings.Builder
	var refs []VarRef
	for i := 0; i < len(s); {
		c := s[i]
		if c != '$' {
			b.WriteByte(c)
			i++
			continue
		}
		if i+1 >= len(s) {
			b.WriteByte('$')
			i++
			continue
		}
		next := s[i+1]
		switch {
		case next == '$':
			b.WriteByte('$')
			i += 2
		case next == '{':
			end, err := matchBrace(s, i+2)
			if err != nil {
				return "", refs, &InterpError{Offset: i, Msg: err.Error()}
			}
			val, r, err := evalBraced(s[i+2:end], lookup)
			refs = append(refs, r...)
			if err != nil {
				return "", refs, &InterpError{Offset: i, Msg: err.Error()}
			}
			b.WriteString(val)
			i = end + 1
		case isNameStart(next):
			j := i + 1
			for j < len(s) && isNameChar(s[j]) {
				j++
			}
			name := s[i+1 : j]
			v, ok := lookup(name)
			refs = append(refs, VarRef{Name: name, Set: ok, Problem: !ok})
			b.WriteString(v)
			i = j
		default:
			b.WriteByte('$')
			i++
		}
	}
	return b.String(), refs, nil
}

// matchBrace returns the index of the "}" closing a "${" whose body starts at start.
func matchBrace(s string, start int) (int, error) {
	depth := 1
	for i := start; i < len(s); i++ {
		switch {
		case s[i] == '$' && i+1 < len(s) && s[i+1] == '$':
			i++
		case s[i] == '$' && i+1 < len(s) && s[i+1] == '{':
			depth++
			i++
		case s[i] == '}':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return 0, fmt.Errorf("unclosed \"${\"")
}

var ops = []string{":-", ":?", ":+", "-", "?", "+"}

func evalBraced(body string, lookup Lookup) (string, []VarRef, error) {
	if body == "" || !isNameStart(body[0]) {
		return "", nil, fmt.Errorf("invalid variable name in \"${%s}\"", body)
	}
	j := 1
	for j < len(body) && isNameChar(body[j]) {
		j++
	}
	name, rest := body[:j], body[j:]
	op, word := "", ""
	if rest != "" {
		for _, o := range ops {
			if strings.HasPrefix(rest, o) {
				op, word = o, rest[len(o):]
				break
			}
		}
		if op == "" {
			return "", nil, fmt.Errorf("invalid modifier in \"${%s}\"", body)
		}
	}
	v, set := lookup(name)
	nonEmpty := set && v != ""
	ref := VarRef{Name: name, Op: op, Set: set}
	expandWord := func() (string, []VarRef, error) {
		w, r, err := Interpolate(word, lookup)
		return w, append([]VarRef{ref}, r...), err
	}
	switch op {
	case "":
		ref.Problem = !set
		return v, []VarRef{ref}, nil
	case ":-":
		if nonEmpty {
			return v, []VarRef{ref}, nil
		}
		return expandWord()
	case "-":
		if set {
			return v, []VarRef{ref}, nil
		}
		return expandWord()
	case ":?":
		ref.Problem = !nonEmpty
		return v, []VarRef{ref}, nil
	case "?":
		ref.Problem = !set
		return v, []VarRef{ref}, nil
	case ":+":
		if nonEmpty {
			return expandWord()
		}
		return "", []VarRef{ref}, nil
	default: // "+"
		if set {
			return expandWord()
		}
		return "", []VarRef{ref}, nil
	}
}

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isNameChar(c byte) bool { return isNameStart(c) || (c >= '0' && c <= '9') }
