package rules

import (
	"strings"
	"testing"

	"github.com/tatitoxt/compose-doctor/internal/compose"
)

func optsProfiles(p ...string) compose.Options { return compose.Options{Profiles: p} }

func TestDependencyRules(t *testing.T) {
	runCases(t, []string{"CD401", "CD402", "CD403"}, []ruleCase{
		{name: "healthy dependency", files: map[string]string{
			"compose.yaml": "services:\n  api:\n    image: x\n    depends_on:\n      db:\n        condition: service_healthy\n  db:\n    image: postgres\n    healthcheck:\n      test: [\"CMD\", \"pg_isready\"]\n"}},
		{name: "short syntax is advisory", files: map[string]string{
			"compose.yaml": "services:\n  api:\n    image: x\n    depends_on: [db]\n  db:\n    image: postgres\n"},
			want: []string{"CD402@compose.yaml:4"}},
		{name: "service_started is advisory", files: map[string]string{
			"compose.yaml": "services:\n  api:\n    image: x\n    depends_on:\n      db: {condition: service_started}\n  db: {image: postgres}\n"},
			want: []string{"CD402@compose.yaml:5"}},
		{name: "completed successfully is fine", files: map[string]string{
			"compose.yaml": "services:\n  api:\n    image: x\n    depends_on:\n      migrate: {condition: service_completed_successfully}\n  migrate: {image: m}\n"}},
		{name: "undefined dependency", files: map[string]string{
			"compose.yaml": "services:\n  api:\n    image: x\n    depends_on: [databse]\n"},
			want: []string{"CD401@compose.yaml:4"}},
		{name: "optional undefined dependency", files: map[string]string{
			"compose.yaml": "services:\n  api:\n    image: x\n    depends_on:\n      cache: {condition: service_started, required: false}\n"}},
		{name: "healthy without healthcheck", files: map[string]string{
			"compose.yaml": "services:\n  api:\n    image: x\n    depends_on:\n      db: {condition: service_healthy}\n  db: {image: postgres}\n"},
			want: []string{"CD403@compose.yaml:5"}},
		{name: "healthy with disabled healthcheck", files: map[string]string{
			"compose.yaml": "services:\n  api:\n    image: x\n    depends_on:\n      db: {condition: service_healthy}\n  db:\n    image: postgres\n    healthcheck: {test: [\"NONE\"]}\n"},
			want: []string{"CD403@compose.yaml:5"}},
		{name: "dependency in inactive profile is not undefined", files: map[string]string{
			"compose.yaml": "services:\n  api:\n    image: x\n    depends_on: [mock]\n  mock: {image: m, profiles: [test]}\n"}},
	})
}

func TestReadinessSuggestion(t *testing.T) {
	f := findOne(t, map[string]string{"compose.yaml": "services:\n  api:\n    image: x\n    depends_on: [db]\n  db:\n    image: postgres\n    healthcheck: {test: [\"CMD\", \"pg_isready\"]}\n"}, nil, "CD402")
	if f.Severity != Info || !strings.Contains(f.Suggestion, "already has a healthcheck") {
		t.Errorf("unexpected %+v", f)
	}
}
