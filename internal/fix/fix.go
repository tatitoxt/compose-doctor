// Package fix applies the textual rewrites behind `compose-doctor check --fix`.
// Only findings from rules marked Fixable are touched, and only when the fix
// cannot break a valid file (removing the obsolete top-level `version:` key).
// The rules package stays read-only; all file mutation lives here.
package fix

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tatitoxt/compose-doctor/internal/rules"
)

// Result describes one applied rewrite.
type Result struct {
	RuleID string `json:"rule_id"`
	File   string `json:"file"`
	Line   int    `json:"line"`
}

// A top-level `version:` key: no indentation, quoted or plain scalar value.
var versionKeyLine = regexp.MustCompile(`^version\s*:`)

// Apply rewrites the compose files referenced by fixable findings under
// projectDir. It returns the applied fixes and the findings it could not fix
// (non-fixable rules, or locations whose line no longer matches the expected
// key — the file may have changed since the scan).
func Apply(projectDir string, findings []rules.Finding) (applied []Result, remaining []rules.Finding, err error) {
	byFile := map[string][]int{}
	fixed := map[int]bool{}
	for i, f := range findings {
		r, ok := rules.Lookup(f.RuleID)
		if !ok || !r.Fixable || f.Location.Line <= 0 {
			continue
		}
		byFile[f.Location.File] = append(byFile[f.Location.File], i)
	}
	for file, idxs := range byFile {
		path := file
		if !filepath.IsAbs(path) {
			path = filepath.Join(projectDir, filepath.FromSlash(file))
		}
		// Never write outside the project directory (e.g. an `include:` path
		// reported as ../other-compose.yaml).
		if rel, relErr := filepath.Rel(projectDir, path); relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			continue
		}
		lines := strings.SplitAfter(string(data), "\n")
		remove := map[int]bool{}
		for _, i := range idxs {
			line := findings[i].Location.Line - 1
			if line < 0 || line >= len(lines) || !versionKeyLine.MatchString(lines[line]) {
				continue
			}
			remove[line] = true
			fixed[i] = true
			applied = append(applied, Result{RuleID: findings[i].RuleID, File: file, Line: findings[i].Location.Line})
		}
		if len(remove) == 0 {
			continue
		}
		var out strings.Builder
		for i, l := range lines {
			if !remove[i] {
				out.WriteString(l)
			}
		}
		if werr := os.WriteFile(path, []byte(out.String()), 0o644); werr != nil {
			return nil, nil, fmt.Errorf("writing %s: %w", file, werr)
		}
	}
	sort.Slice(applied, func(i, j int) bool {
		if applied[i].File != applied[j].File {
			return applied[i].File < applied[j].File
		}
		return applied[i].Line < applied[j].Line
	})
	for i, f := range findings {
		if !fixed[i] {
			remaining = append(remaining, f)
		}
	}
	return applied, remaining, nil
}
