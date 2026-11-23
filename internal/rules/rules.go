// Package rules implements the Compose Doctor checks. Every rule receives a
// loaded project and returns findings; rules never touch the network, run
// processes or modify files, and evidence never contains variable values.
package rules

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tatitoxt/compose-doctor/internal/compose"
)

// Severity of a finding.
type Severity int

// Severities, ordered from least to most severe.
const (
	Info Severity = iota + 1
	Warning
	Error
)

func (s Severity) String() string {
	switch s {
	case Info:
		return "info"
	case Warning:
		return "warning"
	case Error:
		return "error"
	default:
		return "unknown"
	}
}

// MarshalJSON encodes the severity as its name.
func (s Severity) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }

// UnmarshalJSON decodes a severity name.
func (s *Severity) UnmarshalJSON(b []byte) error {
	var name string
	if err := json.Unmarshal(b, &name); err != nil {
		return err
	}
	v, err := ParseSeverity(name)
	*s = v
	return err
}

// ParseSeverity parses "info", "warning" or "error".
func ParseSeverity(name string) (Severity, error) {
	switch strings.ToLower(name) {
	case "info":
		return Info, nil
	case "warning":
		return Warning, nil
	case "error":
		return Error, nil
	}
	return 0, fmt.Errorf("unknown severity %q (want info, warning or error)", name)
}

// Finding is one diagnosed problem.
type Finding struct {
	RuleID     string            `json:"rule_id"`
	Rule       string            `json:"rule"`
	Severity   Severity          `json:"severity"`
	Message    string            `json:"message"`
	Location   compose.Location  `json:"location"`
	Evidence   map[string]string `json:"evidence,omitempty"`
	Suggestion string            `json:"suggestion"`
}

// Rule is a single check.
type Rule struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Severity Severity `json:"severity"`
	Summary  string   `json:"summary"`
	// Fixable marks rules whose findings compose-doctor can rewrite safely
	// (see check --fix); only removals that cannot break a valid file qualify.
	Fixable bool `json:"fixable,omitempty"`
	check   func(Rule, *compose.Project) []Finding
}

func (r Rule) finding(loc compose.Location, msg, suggestion string, evidence map[string]string) Finding {
	return Finding{RuleID: r.ID, Rule: r.Name, Severity: r.Severity, Message: msg, Location: loc, Evidence: evidence, Suggestion: suggestion}
}

// ParseError is reported when the project cannot be analysed. It has no
// check function: the CLI produces it from a compose.LoadError.
var ParseError = Rule{ID: "CD001", Name: "invalid-input", Severity: Error,
	Summary: "A Compose file is missing, unreadable, empty, not valid YAML or exceeds input limits."}

// All returns every rule, sorted by ID.
func All() []Rule {
	all := []Rule{ParseError}
	all = append(all, structureRules()...)
	all = append(all, fileRules()...)
	all = append(all, variableRules()...)
	all = append(all, portRules()...)
	all = append(all, dependencyRules()...)
	all = append(all, referenceRules()...)
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	return all
}

// Lookup returns the rule with the given ID, including advisory rules
// (opt-in at run time but still real rules with metadata and fixability).
func Lookup(id string) (Rule, bool) {
	for _, r := range append(All(), opsRules()...) {
		if r.ID == strings.ToUpper(id) {
			return r, true
		}
	}
	return Rule{}, false
}

// Run applies every rule not listed in disabled and returns sorted findings.
// Advisory rules (CD6xx) only run when `advisory` is true -- they flag missing
// operational hardening, not defects, so they are opt-in via --advisory.
func Run(p *compose.Project, disabled map[string]bool, advisory ...bool) []Finding {
	set := All()
	if len(advisory) > 0 && advisory[0] {
		set = append(set, opsRules()...)
	}
	var out []Finding
	for _, r := range set {
		if r.check == nil || disabled[r.ID] {
			continue
		}
		out = append(out, r.check(r, p)...)
	}
	Sort(out, p.Files)
	return out
}

// FromLoadError converts a load error into a CD001 finding.
func FromLoadError(err *compose.LoadError) Finding {
	return ParseError.finding(err.Loc, err.Msg,
		"Fix the file so it can be read; `docker compose config` shows the same input as Docker Compose sees it. Check indentation (spaces, not tabs), quoting and the selected -f files.",
		nil)
}

// Sort orders findings by file (in load order), line, column, rule and message.
func Sort(fs []Finding, files []string) {
	order := map[string]int{}
	for i, f := range files {
		order[f] = i + 1
	}
	rank := func(f string) int {
		if r, ok := order[f]; ok {
			return r
		}
		return len(files) + 1
	}
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if ra, rb := rank(a.Location.File), rank(b.Location.File); ra != rb {
			return ra < rb
		}
		if a.Location.File != b.Location.File {
			return a.Location.File < b.Location.File
		}
		if a.Location.Line != b.Location.Line {
			return a.Location.Line < b.Location.Line
		}
		if a.Location.Column != b.Location.Column {
			return a.Location.Column < b.Location.Column
		}
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		return a.Message < b.Message
	})
}

func quote(s string) string { return fmt.Sprintf("%q", s) }
