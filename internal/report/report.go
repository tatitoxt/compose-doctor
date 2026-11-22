// Package report renders findings as human-readable text or stable JSON.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/tatitoxt/compose-doctor/internal/rules"
)

// SchemaVersion is incremented on any incompatible change to the JSON output.
const SchemaVersion = 1

// Report is the JSON document written by `check --format json`.
type Report struct {
	SchemaVersion int             `json:"schema_version"`
	Tool          string          `json:"tool"`
	Version       string          `json:"version"`
	Files         []string        `json:"files"`
	Profiles      []string        `json:"profiles"`
	Findings      []rules.Finding `json:"findings"`
	Summary       Summary         `json:"summary"`

	// Dir is the absolute project directory; used by SARIF to make artifact
	// URIs relative to the working directory. Not serialised.
	Dir string `json:"-"`
}

// Summary counts findings by severity.
type Summary struct {
	Total   int `json:"total"`
	Error   int `json:"error"`
	Warning int `json:"warning"`
	Info    int `json:"info"`
}

// New builds a report; nil slices are normalised to empty arrays.
func New(version string, files, profiles []string, findings []rules.Finding) Report {
	r := Report{SchemaVersion: SchemaVersion, Tool: "compose-doctor", Version: version,
		Files: files, Profiles: profiles, Findings: findings}
	if r.Files == nil {
		r.Files = []string{}
	}
	if r.Profiles == nil {
		r.Profiles = []string{}
	}
	if r.Findings == nil {
		r.Findings = []rules.Finding{}
	}
	for _, f := range findings {
		r.Summary.Total++
		switch f.Severity {
		case rules.Error:
			r.Summary.Error++
		case rules.Warning:
			r.Summary.Warning++
		case rules.Info:
			r.Summary.Info++
		}
	}
	return r
}

// JSON writes the report as indented JSON followed by a newline.
func JSON(w io.Writer, r Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}

// Human writes a plain-text report. Output contains no colour codes so it
// is stable in CI logs and golden files.
func Human(w io.Writer, r Report) error {
	var b strings.Builder
	if len(r.Files) > 0 {
		fmt.Fprintf(&b, "Checked %s\n", strings.Join(r.Files, ", "))
	}
	if len(r.Findings) == 0 {
		b.WriteString("No findings.\n")
		_, err := io.WriteString(w, b.String())
		return err
	}
	b.WriteString("\n")
	for _, f := range r.Findings {
		fmt.Fprintf(&b, "%s  %-7s  %s  %s\n", f.Location, f.Severity, f.RuleID, f.Message)
		fmt.Fprintf(&b, "    hint: %s\n", f.Suggestion)
	}
	fmt.Fprintf(&b, "\n%d %s (%d error, %d warning, %d info)\n",
		r.Summary.Total, plural(r.Summary.Total, "finding", "findings"), r.Summary.Error, r.Summary.Warning, r.Summary.Info)
	_, err := io.WriteString(w, b.String())
	return err
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
