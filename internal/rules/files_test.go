package rules

import (
	"strings"
	"testing"
)

func TestFileRules(t *testing.T) {
	runCases(t, []string{"CD101", "CD102", "CD103"}, []ruleCase{
		{name: "all present", files: map[string]string{
			"compose.yaml":   "services:\n  api:\n    build: ./api\n    env_file: [./api.env]\n    volumes: [\"./data:/data\", \"/abs/elsewhere:/x\"]\n",
			"api/Dockerfile": "FROM scratch\n", "api.env": "A=1\n", "data/": ""}},
		{name: "missing env_file short", files: map[string]string{"compose.yaml": "services:\n  api:\n    image: x\n    env_file: ./missing.env\n"},
			want: []string{"CD101@compose.yaml:4"}},
		{name: "env_file list and directory", files: map[string]string{
			"compose.yaml": "services:\n  api:\n    image: x\n    env_file:\n      - ./ok.env\n      - ./config\n      - ./nope.env\n",
			"ok.env":       "", "config/": ""},
			want: []string{"CD101@compose.yaml:6", "CD101@compose.yaml:7"}},
		{name: "optional env_file", files: map[string]string{"compose.yaml": "services:\n  api:\n    image: x\n    env_file:\n      - path: ./local.env\n        required: false\n"}},
		{name: "env_file with unresolved variable skipped", files: map[string]string{"compose.yaml": "services:\n  api:\n    image: x\n    env_file: ./${STAGE}.env\n"}},
		{name: "env_file resolved via .env", files: map[string]string{"compose.yaml": "services:\n  api:\n    image: x\n    env_file: ./${STAGE}.env\n", ".env": "STAGE=dev\n"},
			want: []string{"CD101@compose.yaml:4"}},
		{name: "missing build context", files: map[string]string{"compose.yaml": "services:\n  api:\n    build:\n      context: ./backend\n"},
			want: []string{"CD102@compose.yaml:4"}},
		{name: "missing default Dockerfile", files: map[string]string{"compose.yaml": "services:\n  api:\n    build: ./api\n", "api/main.go": ""},
			want: []string{"CD102@compose.yaml:3"}},
		{name: "containerfile alone is not the default", files: map[string]string{"compose.yaml": "services:\n  api:\n    build: ./api\n", "api/Containerfile": ""},
			want: []string{"CD102@compose.yaml:3"}},
		{name: "containerfile via dockerfile key accepted", files: map[string]string{
			"compose.yaml":      "services:\n  api:\n    build:\n      context: ./api\n      dockerfile: Containerfile\n",
			"api/Containerfile": ""}},
		{name: "custom dockerfile relative to context", files: map[string]string{
			"compose.yaml":              "services:\n  api:\n    build:\n      context: ./api\n      dockerfile: docker/Dockerfile.dev\n  web:\n    build:\n      context: ./api\n      dockerfile: Dockerfile.prod\n",
			"api/docker/Dockerfile.dev": ""},
			want: []string{"CD102@compose.yaml:9"}},
		{name: "remote and inline contexts skipped", files: map[string]string{
			"compose.yaml": "services:\n  a:\n    build: https://github.com/example/repo.git#main\n  b:\n    build:\n      context: .\n      dockerfile_inline: |\n        FROM scratch\n"}},
		{name: "missing relative bind short and long", files: map[string]string{
			"compose.yaml": "services:\n  api:\n    image: x\n    volumes:\n      - ./conf:/etc/conf:ro\n      - type: bind\n        source: ../shared\n        target: /shared\n      - data:/data\n      - /var/run/docker.sock:/var/run/docker.sock\nvolumes:\n  data: {}\n"},
			want: []string{"CD103@compose.yaml:5", "CD103@compose.yaml:7"}},
	})
}

func TestEnvFileSuggestsExampleTemplate(t *testing.T) {
	f := findOne(t, map[string]string{
		"compose.yaml":    "services:\n  api:\n    image: x\n    env_file: ./api.env\n",
		"api.env.example": "A=\n"}, nil, "CD101")
	if !strings.Contains(f.Suggestion, `copy "./api.env.example" to "./api.env"`) {
		t.Errorf("suggestion: %s", f.Suggestion)
	}
	if f.Evidence["path"] != "./api.env" || f.Evidence["service"] != "api" {
		t.Errorf("evidence: %v", f.Evidence)
	}
}
