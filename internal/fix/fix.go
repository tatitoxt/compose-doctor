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

// A top-level `version:` key with a single-line scalar value and no anchor
// or block continuation (multiline/anchored forms are left alone).
var versionKeyLine = regexp.MustCompile(`^version\s*:\s*('[^']*'|"[^"]*"|[^\s&|>{][^\s#]*)\s*(#.*)?$`)

// A service key line inside `services:` (`  web:` — indented, scalar key,
// nothing after the colon except an optional comment).
var serviceKeyLine = regexp.MustCompile(`^(\s+)\S[^:]*:\s*(#.*)?$`)

// A block-mapping entry (`key:` or `key: value`), used to confirm the line
// after a service key can hold an inserted entry.
var blockEntryLine = regexp.MustCompile(`^\s+[^\s{\[-][^:]*:(\s|$)`)
var restartKeyLine = regexp.MustCompile(`^\s+restart\s*:`)

type op struct {
	line       int // 0-based index the op anchors to
	remove     bool
	insertText string
}

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
		// Resolve symlinks on both sides before the boundary check so a
		// project symlink cannot escape into an external file.
		realDir, _ := filepath.EvalSymlinks(projectDir)
		realPath, _ := filepath.EvalSymlinks(path)
		if realPath == "" {
			realPath = path
		}
		if rel, relErr := filepath.Rel(realDir, realPath); relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			continue
		}
		lines := strings.SplitAfter(string(data), "\n")
		var ops []op
		for _, i := range idxs {
			o, ok := planOp(findings[i], lines)
			if !ok {
				continue
			}
			ops = append(ops, o)
			fixed[i] = true
			applied = append(applied, Result{RuleID: findings[i].RuleID, File: file, Line: findings[i].Location.Line})
		}
		if len(ops) == 0 {
			continue
		}
		out := rewrite(lines, ops)
		if werr := os.WriteFile(path, []byte(out), 0o644); werr != nil {
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

// planOp maps a fixable finding to the textual op it needs, verifying the
// anchored line still looks like what the rule reported. Returns false when
// the file drifted or the shape is not safely rewritable.
func planOp(f rules.Finding, lines []string) (op, bool) {
	line := f.Location.Line - 1
	if line < 0 || line >= len(lines) {
		return op{}, false
	}
	switch f.RuleID {
	case "CD003":
		if !versionKeyLine.MatchString(lines[line]) {
			return op{}, false
		}
		return op{line: line, remove: true}, true
	case "CD601":
		// Insert `restart: unless-stopped` inside the service's block mapping:
		// the service key must end the line, and the next non-blank,
		// non-comment line must be a block entry at deeper indent (flow
		// mappings, comments and scalars are left alone).
		m := serviceKeyLine.FindStringSubmatch(lines[line])
		if m == nil {
			return op{}, false
		}
		indent := ""
		for j := line + 1; j < len(lines); j++ {
			body := lines[j]
			t := strings.TrimSpace(body)
			if t == "" {
				continue
			}
			if !strings.HasPrefix(body, m[1]+" ") && !strings.HasPrefix(body, m[1]+"\t") {
				return op{}, false
			}
			if strings.HasPrefix(t, "#") {
				continue
			}
			if restartKeyLine.MatchString(body) {
				return op{}, false // a restart key (even !reset) exists
			}
			if !blockEntryLine.MatchString(body) {
				return op{}, false
			}
			indent = body[:len(body)-len(strings.TrimLeft(body, " \t"))]
			break
		}
		if indent == "" {
			return op{}, false
		}
		return op{line: line, insertText: indent + "restart: unless-stopped\n"}, true
	}
	return op{}, false
}

// rewrite applies removals and insertions; inserts anchor after their line so
// ordering is stable regardless of op order.
func rewrite(lines []string, ops []op) string {
	remove := map[int]bool{}
	insert := map[int][]string{}
	for _, o := range ops {
		if o.remove {
			remove[o.line] = true
		}
		if o.insertText != "" {
			insert[o.line] = append(insert[o.line], o.insertText)
		}
	}
	var out strings.Builder
	for i, l := range lines {
		if !remove[i] {
			out.WriteString(l)
		}
		for _, t := range insert[i] {
			out.WriteString(t)
		}
	}
	return out.String()
}
