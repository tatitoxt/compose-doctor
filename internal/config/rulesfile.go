package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tatitoxt/compose-doctor/internal/rules"
)

// RulesFileName is the per-project config file `check` picks up automatically.
const RulesFileName = "compose-doctor.json"

// rulesFile mirrors the on-disk JSON. "off" disables a rule.
type rulesFile struct {
	Severity map[string]string `json:"severity"`
}

// maxRulesFileSize bounds a per-project rules file.
const maxRulesFileSize = 1 << 20

// ApplyRulesFile loads the per-project rules file: the explicit --config path
// when given (a missing or invalid file is an error), else the default in the
// project directory when present. With -f, the project directory is the first
// Compose file's parent.
func (c *Check) ApplyRulesFile() error {
	path := c.ConfigPath
	if path == "" {
		dir := c.Path
		if len(c.Files) > 0 {
			f := c.Files[0]
			if !filepath.IsAbs(f) {
				f = filepath.Join(c.Path, f)
			}
			dir = filepath.Dir(f)
		}
		path = defaultRulesFile(dir)
		if path == "" {
			return nil
		}
	}
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot read rules file %s: %w", path, err)
	}
	if !fi.Mode().IsRegular() || fi.Size() > maxRulesFileSize {
		return fmt.Errorf("cannot read rules file %s: not a regular file or over %d bytes", path, maxRulesFileSize)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read rules file %s: %w", path, err)
	}
	var rf rulesFile
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rf); err != nil {
		return fmt.Errorf("invalid rules file %s: %w", path, err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("invalid rules file %s: trailing data after the JSON object", path)
	}
	if c.Severities == nil {
		c.Severities = map[string]rules.Severity{}
	}
	for id, val := range rf.Severity {
		id = strings.ToUpper(strings.TrimSpace(id))
		if _, ok := rules.Lookup(id); !ok {
			return fmt.Errorf("%s: unknown rule %q (see `compose-doctor rules`)", path, id)
		}
		if id == rules.ParseError.ID {
			return fmt.Errorf("%s: CD001 cannot be disabled or re-severitied", path)
		}
		if strings.EqualFold(strings.TrimSpace(val), "off") {
			c.Disabled[id] = true
			continue
		}
		sev, err := rules.ParseSeverity(val)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		c.Severities[id] = sev
	}
	return nil
}

// defaultRulesFile returns <dir>/compose-doctor.json when it exists. dir is the
// project directory: path when it is a directory, else its parent.
func defaultRulesFile(path string) string {
	dir := path
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		dir = filepath.Dir(path)
	}
	candidate := filepath.Join(dir, RulesFileName)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}
