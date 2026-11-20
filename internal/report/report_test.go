package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tatitoxt/compose-doctor/internal/compose"
	"github.com/tatitoxt/compose-doctor/internal/rules"
)

func sample() []rules.Finding {
	return []rules.Finding{
		{RuleID: "CD101", Rule: "env-file-not-found", Severity: rules.Error, Message: "m1",
			Location: compose.Location{File: "compose.yaml", Line: 4, Column: 15}, Evidence: map[string]string{"service": "api"}, Suggestion: "s1"},
		{RuleID: "CD402", Rule: "depends-on-without-readiness", Severity: rules.Info, Message: "m2",
			Location: compose.Location{File: "compose.yaml", Line: 9}, Suggestion: "s2"},
	}
}

func TestJSONRoundTripAndShape(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, New("test", []string{"compose.yaml"}, nil, sample())); err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	dec.DisallowUnknownFields()
	var got Report
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("decode: %v\n%s", err, buf.String())
	}
	if got.SchemaVersion != 1 || got.Summary != (Summary{Total: 2, Error: 1, Info: 1}) || got.Findings[0].Severity != rules.Error {
		t.Errorf("unexpected report %+v", got)
	}
	if !strings.Contains(buf.String(), `"severity": "error"`) || !strings.Contains(buf.String(), `"profiles": []`) {
		t.Errorf("unexpected JSON:\n%s", buf.String())
	}
}

func TestEmptyFindingsIsArray(t *testing.T) {
	var buf bytes.Buffer
	_ = JSON(&buf, New("test", nil, nil, nil))
	if !strings.Contains(buf.String(), `"findings": []`) {
		t.Errorf("findings must be an empty array:\n%s", buf.String())
	}
}

func TestHuman(t *testing.T) {
	var buf bytes.Buffer
	_ = Human(&buf, New("test", []string{"compose.yaml"}, nil, sample()))
	want := "Checked compose.yaml\n\n" +
		"compose.yaml:4:15  error    CD101  m1\n    hint: s1\n" +
		"compose.yaml:9  info     CD402  m2\n    hint: s2\n" +
		"\n2 findings (1 error, 0 warning, 1 info)\n"
	if buf.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", buf.String(), want)
	}
	buf.Reset()
	_ = Human(&buf, New("test", []string{"compose.yaml"}, nil, nil))
	if buf.String() != "Checked compose.yaml\nNo findings.\n" {
		t.Errorf("no findings output: %q", buf.String())
	}
}
