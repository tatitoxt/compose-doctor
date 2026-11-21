package rules

import "testing"

func TestStructureRules(t *testing.T) {
	runCases(t, []string{"CD002", "CD003", "CD004"}, []ruleCase{
		{name: "valid", files: map[string]string{"compose.yaml": "services:\n  web:\n    image: nginx\n"}},
		{name: "no image or build", files: map[string]string{"compose.yaml": "services:\n  web:\n    ports: [\"80\"]\n  ok:\n    build: .\n"},
			want: []string{"CD002@compose.yaml:2"}},
		{name: "null service", files: map[string]string{"compose.yaml": "services:\n  web:\n"}, want: []string{"CD002@compose.yaml:2"}},
		{name: "extends is not flagged", files: map[string]string{"compose.yaml": "services:\n  web:\n    extends: {file: base.yaml, service: web}\n", "base.yaml": "services:\n  web:\n    image: nginx\n"}},
		{name: "inactive profile not flagged", files: map[string]string{"compose.yaml": "services:\n  dbg:\n    profiles: [debug]\n"}},
		{name: "image from override", files: map[string]string{
			"compose.yaml": "services:\n  web: {}\n", "compose.override.yaml": "services:\n  web: {image: nginx}\n"}},
		{name: "version key", files: map[string]string{"compose.yaml": "version: '3.9'\nservices:\n  web: {image: x}\n"}, want: []string{"CD003@compose.yaml:1"}},
		{name: "duplicate key", files: map[string]string{"compose.yaml": "services:\n  web:\n    image: x\n    environment: {A: 1}\n    environment: {B: 2}\n"},
			want: []string{"CD004@compose.yaml:5"}},
		{name: "merge key is not a duplicate", files: map[string]string{"compose.yaml": "x: &x {image: a}\ny: &y {user: b}\nservices:\n  web:\n    <<: *x\n    <<: *y\n"}},
	})
}
