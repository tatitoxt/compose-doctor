// Baseline files let a legacy project silence its pre-existing findings while
// keeping new violations visible. `check --write-baseline` snapshots the
// current findings to <project>/compose-doctor-baseline.json (or
// --baseline PATH); later runs suppress findings that match a recorded entry.
package rules

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// BaselineFileName is the auto-detected baseline in the project directory.
const BaselineFileName = "compose-doctor-baseline.json"

// BaselineEntry records one known finding. Matching is on rule, file and
// message — the line is kept for human readers but not matched, so edits
// above an excused block do not un-suppress it.
type BaselineEntry struct {
	RuleID  string `json:"rule_id"`
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

// LoadBaseline reads a baseline file.
func LoadBaseline(path string) ([]BaselineEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []BaselineEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for i, e := range entries {
		if e.RuleID == "" || e.File == "" {
			return nil, fmt.Errorf("%s: entry %d lacks rule_id or file", path, i)
		}
	}
	return entries, nil
}

// WriteBaseline serializes findings as baseline entries, sorted for a stable diff.
func WriteBaseline(path string, findings []Finding) error {
	entries := BaselineFromFindings(findings)
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// BaselineFromFindings converts findings to sorted baseline entries.
func BaselineFromFindings(findings []Finding) []BaselineEntry {
	entries := make([]BaselineEntry, 0, len(findings))
	for _, f := range findings {
		entries = append(entries, BaselineEntry{
			RuleID:  f.RuleID,
			File:    f.Location.File,
			Line:    f.Location.Line,
			Message: f.Message,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		return a.Message < b.Message
	})
	return entries
}

// ApplyBaseline drops findings matching a baseline entry and returns the kept
// findings plus the number suppressed.
// locRe strips "file:line:col" fragments so a baseline match survives lines
// moving when unrelated content is inserted above them.
var locRe = regexp.MustCompile(`\S+:\d+(:\d+)?`)

func normMsg(s string) string { return locRe.ReplaceAllString(s, "<loc>") }

func ApplyBaseline(findings []Finding, entries []BaselineEntry) ([]Finding, int) {
	known := map[string]int{}
	for _, e := range entries {
		known[e.RuleID+"\x00"+e.File+"\x00"+normMsg(e.Message)]++
	}
	var kept []Finding
	suppressed := 0
	for _, f := range findings {
		key := f.RuleID + "\x00" + f.Location.File + "\x00" + normMsg(f.Message)
		if known[key] > 0 {
			known[key]--
			suppressed++
			continue
		}
		kept = append(kept, f)
	}
	return kept, suppressed
}

// StaleEntries lists baseline entries that no finding matched (the suppressed
// code was fixed or moved) — reported so baselines do not accumulate rot.
func StaleEntries(entries []BaselineEntry, findings []Finding) []BaselineEntry {
	seen := map[string]bool{}
	for _, f := range findings {
		seen[f.RuleID+"\x00"+f.Location.File+"\x00"+normMsg(f.Message)] = true
	}
	var stale []BaselineEntry
	for _, e := range entries {
		if !seen[e.RuleID+"\x00"+e.File+"\x00"+normMsg(e.Message)] {
			stale = append(stale, e)
		}
	}
	return stale
}

// FormatStale renders stale entries for stderr.
func FormatStale(entries []BaselineEntry) string {
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "%s:%d %s %s\n", e.File, e.Line, e.RuleID, e.Message)
	}
	return b.String()
}
