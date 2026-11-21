package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tatitoxt/compose-doctor/internal/rules"
)

func TestSARIFValidLog(t *testing.T) {
	var buf bytes.Buffer
	if err := SARIF(&buf, New("test", []string{"compose.yaml"}, nil, sample())); err != nil {
		t.Fatal(err)
	}
	var log struct {
		Version string `json:"version"`
		Schema  string `json:"$schema"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Name           string `json:"name"`
					Version        string `json:"version"`
					InformationURI string `json:"informationUri"`
					Rules          []struct {
						ID               string `json:"id"`
						ShortDescription struct {
							Text string `json:"text"`
						} `json:"shortDescription"`
						DefaultConfiguration struct {
							Level string `json:"level"`
						} `json:"defaultConfiguration"`
						HelpURI string `json:"helpUri"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID    string `json:"ruleId"`
				RuleIndex int    `json:"ruleIndex"`
				Level     string `json:"level"`
				Message   struct {
					Text string `json:"text"`
				} `json:"message"`
				Locations []struct {
					PhysicalLocation struct {
						ArtifactLocation struct {
							URI string `json:"uri"`
						} `json:"artifactLocation"`
						Region *struct {
							StartLine   int `json:"startLine"`
							StartColumn int `json:"startColumn"`
						} `json:"region"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(buf.Bytes(), &log); err != nil {
		t.Fatalf("SARIF is not valid JSON: %v\n%s", err, buf.String())
	}
	if log.Version != "2.1.0" || log.Schema == "" || len(log.Runs) != 1 {
		t.Fatalf("not a SARIF 2.1.0 log: version %q schema %q runs %d", log.Version, log.Schema, len(log.Runs))
	}
	driver := log.Runs[0].Tool.Driver
	if driver.Name != "compose-doctor" || driver.Version != "test" || driver.InformationURI == "" {
		t.Errorf("bad driver: %+v", driver)
	}
	if len(driver.Rules) != len(rules.All()) {
		t.Errorf("rule catalog %d entries, want %d", len(driver.Rules), len(rules.All()))
	}
	results := log.Runs[0].Results
	if len(results) != 2 {
		t.Fatalf("results %d, want 2", len(results))
	}
	if results[0].RuleID != "CD101" || results[0].Level != "error" {
		t.Errorf("first result %+v", results[0])
	}
	if !strings.Contains(results[0].Message.Text, "m1") || !strings.Contains(results[0].Message.Text, "s1") {
		t.Errorf("message lacks finding text and hint: %q", results[0].Message.Text)
	}
	loc := results[0].Locations[0].PhysicalLocation
	if loc.ArtifactLocation.URI != "compose.yaml" {
		t.Errorf("artifact URI %q", loc.ArtifactLocation.URI)
	}
	if loc.Region == nil || loc.Region.StartLine != 4 || loc.Region.StartColumn != 15 {
		t.Errorf("region %+v", loc.Region)
	}
	if results[1].Level != "note" {
		t.Errorf("info severity must map to SARIF note, got %q", results[1].Level)
	}
	if results[1].Locations[0].PhysicalLocation.Region == nil || results[1].Locations[0].PhysicalLocation.Region.StartLine != 9 {
		t.Errorf("second result region %+v", results[1].Locations[0].PhysicalLocation.Region)
	}
}

func TestSARIFEmptyResultsIsArray(t *testing.T) {
	var buf bytes.Buffer
	_ = SARIF(&buf, New("test", nil, nil, nil))
	if !strings.Contains(buf.String(), `"results": []`) {
		t.Errorf("results must be an empty array:\n%s", buf.String())
	}
}
