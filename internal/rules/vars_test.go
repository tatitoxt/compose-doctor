package rules

import (
	"strings"
	"testing"

	"github.com/tatitoxt/compose-doctor/internal/compose"
)

func TestVariableRules(t *testing.T) {
	runCases(t, []string{"CD201", "CD202", "CD203"}, []ruleCase{
		{name: "defined in .env and shell", files: map[string]string{
			"compose.yaml": "services:\n  db:\n    image: \"postgres:${PG_TAG}\"\n    environment:\n      POSTGRES_PASSWORD: ${DB_PASSWORD:?set it}\n",
			".env":         "PG_TAG=16\n"}, environ: []string{"DB_PASSWORD=synthetic"}},
		{name: "undefined grouped", files: map[string]string{
			"compose.yaml": "services:\n  a:\n    image: \"x:${TAG}\"\n  b:\n    image: \"y:$TAG\"\n"},
			want: []string{"CD201@compose.yaml:3"}},
		{name: "defaults and escapes are fine", files: map[string]string{
			"compose.yaml": "services:\n  a:\n    image: \"x:${TAG:-latest}\"\n    command: echo $$HOME ${OPT:+--flag}\n"}},
		{name: "required unset", files: map[string]string{
			"compose.yaml": "services:\n  db:\n    image: postgres\n    environment:\n      POSTGRES_PASSWORD: ${DB_PASSWORD:?DB_PASSWORD must be set}\n"},
			want: []string{"CD202@compose.yaml:5"}},
		{name: "required set but empty", files: map[string]string{
			"compose.yaml": "services:\n  db:\n    image: postgres\n    environment: [\"P=${DB_PASSWORD:?}\"]\n", ".env": "DB_PASSWORD=\n"},
			want: []string{"CD202@compose.yaml:4"}},
		{name: "invalid syntax", files: map[string]string{
			"compose.yaml": "services:\n  a:\n    image: \"x:${TAG\"\n"},
			want: []string{"CD203@compose.yaml:3"}},
		{name: "unused anchor still interpolated", files: map[string]string{
			"compose.yaml": "x-base: &base\n  image: ${BASE_IMAGE}\nservices:\n  a:\n    <<: *base\n"},
			want: []string{"CD201@compose.yaml:2"}},
	})
}

func TestUndefinedVariableSuggestions(t *testing.T) {
	f := findOne(t, map[string]string{
		"compose.yaml": "services:\n  api:\n    image: \"api:${API_TAG}\"\n    env_file: ./api.env\n",
		"api.env":      "API_TAG=synthetic-value\n"}, nil, "CD201")
	if !strings.Contains(f.Suggestion, "env_file only sets the container environment") {
		t.Errorf("expected env_file hint, got %s", f.Suggestion)
	}
	f = findOne(t, map[string]string{
		"compose.yaml": "services:\n  api:\n    image: \"api:${API_TAG}\"\n",
		".env.example": "API_TAG=\n"}, nil, "CD201")
	if !strings.Contains(f.Suggestion, "copy .env.example to .env") {
		t.Errorf("expected .env.example hint, got %s", f.Suggestion)
	}
	f = findOne(t, map[string]string{
		"compose.yaml": "services:\n  api:\n    image: \"api:${API_TAG}\"\n    command: run $API_TAG\n",
		".env.example": "API_TAG=\n", ".env": "OTHER=1\n"}, nil, "CD201")
	if !strings.Contains(f.Suggestion, "missing from .env") || f.Evidence["other_locations"] != "compose.yaml:4:14" {
		t.Errorf("unexpected finding %+v", f)
	}
}

func TestVariableValuesNeverAppear(t *testing.T) {
	const secret = "synthetic-secret-value-123"
	p := load(t, map[string]string{
		"compose.yaml": "services:\n  api:\n    image: \"api:${TAG}\"\n    env_file: ./${SECRET_NAME}.env\n    ports: [\"${PORT}:80\", \"${PORT}:81\"]\n    volumes: [\"${SECRET_NAME}:/data\"]\n",
		".env":         "SECRET_NAME=" + secret + "\nPORT=8080\n"}, compose.Options{})
	seen := map[string]bool{}
	for _, f := range Run(p, nil) {
		seen[f.RuleID] = true
		blob := f.Message + f.Suggestion
		for _, v := range f.Evidence {
			blob += v
		}
		if strings.Contains(blob, secret) || strings.Contains(blob, "8080") {
			t.Errorf("%s leaked a variable value: %s", f.RuleID, blob)
		}
	}
	for _, id := range []string{"CD101", "CD301", "CD501"} {
		if !seen[id] {
			t.Errorf("expected a %s finding to exercise redaction", id)
		}
	}
}
