package report

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tatitoxt/compose-doctor/internal/rules"
)

// SARIF 2.1.0 — the subset code-scanning integrations (GitHub code scanning,
// SARIF viewers) consume: one run, the rule catalog and one result per
// finding with a physical location.

const sarifSchema = "https://json.schemastore.org/sarif-2.1.0.json"

// ToolHomepage is linked from every result's rule metadata.
const ToolHomepage = "https://github.com/tatitoxt/compose-doctor"

type sarifLog struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID                   string           `json:"id"`
	Name                 string           `json:"name"`
	ShortDescription     sarifMessage     `json:"shortDescription"`
	DefaultConfiguration sarifRuleDefault `json:"defaultConfiguration"`
	HelpURI              string           `json:"helpUri"`
}

type sarifRuleDefault struct {
	Level string `json:"level"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	RuleIndex int             `json:"ruleIndex"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifact `json:"artifactLocation"`
	Region           *sarifRegion  `json:"region,omitempty"`
}

type sarifArtifact struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn,omitempty"`
}

func sarifLevel(s rules.Severity) string {
	switch s {
	case rules.Error:
		return "error"
	case rules.Warning:
		return "warning"
	default:
		return "note"
	}
}

// SARIF writes the report as a SARIF 2.1.0 log (one run, every known rule in
// the driver catalog, one result per finding).
func SARIF(w io.Writer, r Report) error {
	catalog := rules.All()
	index := make(map[string]int, len(catalog))
	srules := make([]sarifRule, 0, len(catalog))
	for i, rule := range catalog {
		index[rule.ID] = i
		srules = append(srules, sarifRule{
			ID:               rule.ID,
			Name:             rule.Name,
			ShortDescription: sarifMessage{Text: rule.Summary},
			DefaultConfiguration: sarifRuleDefault{
				Level: sarifLevel(rule.Severity),
			},
			HelpURI: ToolHomepage + "/blob/main/docs/rules.md",
		})
	}
	cwd, _ := os.Getwd()
	uri := func(file string) string {
		abs := file
		if !filepath.IsAbs(abs) && r.Dir != "" {
			abs = filepath.Join(r.Dir, filepath.FromSlash(file))
		}
		if rel, err := filepath.Rel(cwd, abs); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
		return filepath.ToSlash(file)
	}
	results := make([]sarifResult, 0, len(r.Findings))
	for _, f := range r.Findings {
		msg := f.Message
		if f.Suggestion != "" {
			msg += " " + f.Suggestion
		}
		loc := sarifLocation{PhysicalLocation: sarifPhysicalLocation{
			ArtifactLocation: sarifArtifact{URI: uri(f.Location.File)},
		}}
		if f.Location.Line > 0 {
			loc.PhysicalLocation.Region = &sarifRegion{
				StartLine:   f.Location.Line,
				StartColumn: f.Location.Column,
			}
		}
		results = append(results, sarifResult{
			RuleID:    f.RuleID,
			RuleIndex: index[f.RuleID],
			Level:     sarifLevel(f.Severity),
			Message:   sarifMessage{Text: msg},
			Locations: []sarifLocation{loc},
		})
	}
	log := sarifLog{
		Version: "2.1.0",
		Schema:  sarifSchema,
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           "compose-doctor",
				Version:        r.Version,
				InformationURI: ToolHomepage,
				Rules:          srules,
			}},
			Results: results,
		}},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(log)
}
