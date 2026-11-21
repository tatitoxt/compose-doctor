// Inline ignore directives. A Compose file can suppress findings with
// `compose-doctor:` comments:
//
//	service: ...          # compose-doctor:ignore CD102
//	# compose-doctor:ignore CD102 CD401   (governs the next entry's line)
//	# compose-doctor:ignore               (all rules on that line)
//	# compose-doctor:ignore-file          (the whole file)
package rules

import (
	"regexp"
	"sort"
	"strings"

	"github.com/tatitoxt/compose-doctor/internal/compose"
)

var (
	// A directive comment: `#` then `compose-doctor:` then the verb and
	// optional rule IDs (`CD102`, comma- or space-separated).
	directiveRe = regexp.MustCompile(`#\s*compose-doctor:(ignore-file|ignore)\b([ \t]+[A-Za-z0-9, \t]*)?`)
	ruleIDRe    = regexp.MustCompile(`CD\d{3}`)
)

// Ignores are the suppression directives of one Compose file.
type Ignores struct {
	wholeFile bool
	// lines maps a governed source line to the allowed rule IDs; an empty
	// inner map means "all rules".
	lines map[int]map[string]bool
}

// ParseIgnores scans raw Compose text for directives. A directive trailing
// content on the same line governs that line; a directive on its own line
// governs the next non-blank, non-comment line.
func ParseIgnores(text string) Ignores {
	ig := Ignores{lines: map[int]map[string]bool{}}
	src := strings.Split(text, "\n")
	pendingAll := -1 // own-line directive awaiting its governed line
	var pendingIDs map[string]bool
	for i, line := range src {
		ln := i + 1
		m := directiveRe.FindStringSubmatch(line)
		if m == nil {
			if strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "#") && pendingAll >= 0 {
				ig.lines[ln] = pendingIDs
				pendingAll = -1
			}
			continue
		}
		if m[1] == "ignore-file" {
			ig.wholeFile = true
			continue
		}
		ids := map[string]bool{}
		for _, id := range ruleIDRe.FindAllString(m[2], -1) {
			ids[id] = true
		}
		before := line[:strings.Index(line, "#")]
		if strings.TrimSpace(before) != "" {
			// Trailing comment: governs this line.
			if ig.lines[ln] == nil {
				ig.lines[ln] = map[string]bool{}
			}
			for k, v := range ids {
				ig.lines[ln][k] = v
			}
			continue
		}
		if pendingAll >= 0 {
			// Stacked own-line directives merge their ID sets.
			for k, v := range ids {
				pendingIDs[k] = v
			}
			continue
		}
		pendingAll, pendingIDs = ln, ids
	}
	return ig
}

// suppresses reports whether a finding at loc for ruleID is ignored.
func (ig Ignores) suppresses(ruleID string, loc compose.Location) bool {
	if ig.wholeFile {
		return true
	}
	ids, ok := ig.lines[loc.Line]
	if !ok {
		return false
	}
	return len(ids) == 0 || ids[ruleID]
}

// ApplyIgnores drops findings suppressed by each file's directives and
// returns the kept findings plus the number suppressed.
func ApplyIgnores(findings []Finding, files map[string]string) ([]Finding, int) {
	perFile := map[string]Ignores{}
	for path, text := range files {
		perFile[path] = ParseIgnores(text)
	}
	var kept []Finding
	suppressed := 0
	for _, f := range findings {
		ig, ok := perFile[f.Location.File]
		if ok && ig.suppresses(f.RuleID, f.Location) {
			suppressed++
			continue
		}
		kept = append(kept, f)
	}
	return kept, suppressed
}

// IgnoredRuleIDs lists distinct rule IDs named by a file's directives — for
// warning on directives that suppress nothing. (Reserved for a future
// --report-unused-directives flag.)
func (ig Ignores) IgnoredRuleIDs() []string {
	set := map[string]bool{}
	for _, ids := range ig.lines {
		for id := range ids {
			set[id] = true
		}
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
