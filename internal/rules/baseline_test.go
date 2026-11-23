package rules

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBaselineRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, BaselineFileName)
	findings := []Finding{
		{RuleID: "CD301", Message: "conflict a", Location: loc("compose.yml", 9)},
		{RuleID: "CD301", Message: "conflict b", Location: loc("compose.yml", 5)},
	}
	if err := WriteBaseline(path, findings); err != nil {
		t.Fatal(err)
	}
	entries, err := LoadBaseline(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].File != "compose.yml" {
		t.Fatalf("entries %+v", entries)
	}
	// Sorted output: "conflict a" before "conflict b" regardless of line.
	if entries[0].Message != "conflict a" {
		t.Fatalf("unsorted baseline: %v", entries)
	}
}

func TestApplyBaselineMatchesRuleFileMessage(t *testing.T) {
	entries := []BaselineEntry{
		{RuleID: "CD301", File: "compose.yml", Line: 9, Message: "conflict"},
	}
	findings := []Finding{
		// Line drift is tolerated.
		{RuleID: "CD301", Location: loc("compose.yml", 14), Message: "conflict"},
		// Same rule+file but a different message stays.
		{RuleID: "CD301", Location: loc("compose.yml", 20), Message: "other conflict"},
		{RuleID: "CD401", Location: loc("compose.yml", 14), Message: "dep"},
	}
	kept, n := ApplyBaseline(findings, entries)
	if n != 1 || len(kept) != 2 || kept[0].Message != "other conflict" {
		t.Fatalf("kept=%v suppressed=%d", kept, n)
	}
}

func TestStaleEntries(t *testing.T) {
	entries := []BaselineEntry{
		{RuleID: "CD301", File: "compose.yml", Message: "gone"},
		{RuleID: "CD301", File: "compose.yml", Message: "still there"},
	}
	findings := []Finding{{RuleID: "CD301", Location: loc("compose.yml", 4), Message: "still there"}}
	stale := StaleEntries(entries, findings)
	if len(stale) != 1 || stale[0].Message != "gone" {
		t.Fatalf("stale %v", stale)
	}
}

func TestLoadBaselineRejectsMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, BaselineFileName)
	if err := os.WriteFile(path, []byte(`[{"rule_id":"CD301"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBaseline(path); err == nil {
		t.Fatal("entry without file must be rejected")
	}
}
