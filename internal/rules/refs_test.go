package rules

import "testing"

func TestReferenceRules(t *testing.T) {
	runCases(t, []string{"CD501", "CD502"}, []ruleCase{
		{name: "declared", files: map[string]string{
			"compose.yaml": "services:\n  db:\n    image: postgres\n    volumes: [\"pgdata:/var/lib/postgresql/data\", \"/tmp/cache\"]\n    networks: [backend, default]\nvolumes:\n  pgdata:\nnetworks:\n  backend:\n"}},
		{name: "undefined volume short and long", files: map[string]string{
			"compose.yaml": "services:\n  db:\n    image: postgres\n    volumes:\n      - pgdata:/data\n      - type: volume\n        source: logs\n        target: /logs\n"},
			want: []string{"CD501@compose.yaml:5", "CD501@compose.yaml:7"}},
		{name: "declared in override", files: map[string]string{
			"compose.yaml":          "services:\n  db: {image: x, volumes: [\"pgdata:/data\"]}\n",
			"compose.override.yaml": "volumes:\n  pgdata: {}\n"}},
		{name: "undefined networks list and map", files: map[string]string{
			"compose.yaml": "services:\n  a:\n    image: x\n    networks: [front]\n  b:\n    image: x\n    networks:\n      back:\n        aliases: [b]\n"},
			want: []string{"CD502@compose.yaml:4", "CD502@compose.yaml:8"}},
		{name: "network_mode skips networks", files: map[string]string{
			"compose.yaml": "services:\n  a:\n    image: x\n    network_mode: \"service:b\"\n    networks: [front]\n  b: {image: x}\n"}},
	})
}
