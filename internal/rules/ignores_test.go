package rules

import (
	"testing"

	"github.com/tatitoxt/compose-doctor/internal/compose"
)

func loc(file string, line int) compose.Location {
	return compose.Location{File: file, Line: line}
}

func TestParseIgnores_TrailingComment(t *testing.T) {
	ig := ParseIgnores("services:\n  web:\n    image: nginx  # compose-doctor:ignore CD102\n")
	if !ig.suppresses("CD102", loc("compose.yml", 3)) {
		t.Fatal("trailing directive did not suppress its line")
	}
	if ig.suppresses("CD102", loc("compose.yml", 2)) {
		t.Fatal("directive leaked to another line")
	}
	if ig.suppresses("CD401", loc("compose.yml", 3)) {
		t.Fatal("directive suppressed a rule it did not name")
	}
}

func TestParseIgnores_OwnLineGovernsNextEntry(t *testing.T) {
	src := "services:\n  web:\n    # compose-doctor:ignore CD102\n\n    # an ordinary comment\n    image: nginx\n"
	ig := ParseIgnores(src)
	if !ig.suppresses("CD102", loc("compose.yml", 6)) {
		t.Fatal("own-line directive did not govern the next entry line")
	}
}

func TestParseIgnores_StackedDirectivesMerge(t *testing.T) {
	src := "  # compose-doctor:ignore CD102\n  # compose-doctor:ignore CD401\n  image: nginx\n"
	ig := ParseIgnores(src)
	if !ig.suppresses("CD102", loc("compose.yml", 3)) || !ig.suppresses("CD401", loc("compose.yml", 3)) {
		t.Fatal("stacked directives did not merge")
	}
}

func TestParseIgnores_NoIDsMeansAll(t *testing.T) {
	ig := ParseIgnores("  # compose-doctor:ignore\n  image: nginx\n")
	if !ig.suppresses("CD999", loc("compose.yml", 2)) {
		t.Fatal("bare ignore did not suppress all rules")
	}
}

func TestParseIgnores_IgnoreFile(t *testing.T) {
	ig := ParseIgnores("# compose-doctor:ignore-file\nservices: {}\n")
	if !ig.suppresses("CD101", loc("compose.yml", 50)) {
		t.Fatal("ignore-file did not suppress a distant line")
	}
}

func TestApplyIgnores(t *testing.T) {
	files := map[string]string{
		"compose.yml": "services:\n  web:\n    image: nginx  # compose-doctor:ignore CD102\n",
	}
	findings := []Finding{
		{RuleID: "CD102", Location: loc("compose.yml", 3)},
		{RuleID: "CD401", Location: loc("compose.yml", 3)},
		{RuleID: "CD102", Location: loc("compose.yml", 9)},
	}
	kept, suppressed := ApplyIgnores(findings, files)
	if suppressed != 1 || len(kept) != 2 {
		t.Fatalf("kept=%d suppressed=%d, want 2/1", len(kept), suppressed)
	}
	if kept[0].RuleID != "CD401" || kept[1].Location.Line != 9 {
		t.Fatalf("wrong findings kept: %+v", kept)
	}
}
