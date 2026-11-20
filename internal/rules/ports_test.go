package rules

import (
	"strings"
	"testing"
)

func TestParseShortPort(t *testing.T) {
	tests := []struct {
		in   string
		want Binding
		err  string
	}{
		{in: "80", want: Binding{ContainerStart: 80, ContainerEnd: 80, Protocol: "tcp"}},
		{in: "8080:80", want: Binding{HostStart: 8080, HostEnd: 8080, ContainerStart: 80, ContainerEnd: 80, Protocol: "tcp"}},
		{in: "127.0.0.1:8080:80/udp", want: Binding{HostIP: "127.0.0.1", HostStart: 8080, HostEnd: 8080, ContainerStart: 80, ContainerEnd: 80, Protocol: "udp"}},
		{in: "127.0.0.1::80", want: Binding{HostIP: "127.0.0.1", ContainerStart: 80, ContainerEnd: 80, Protocol: "tcp"}},
		{in: "[::1]:8080:80", want: Binding{HostIP: "::1", HostStart: 8080, HostEnd: 8080, ContainerStart: 80, ContainerEnd: 80, Protocol: "tcp"}},
		{in: "9000-9001:9000-9001", want: Binding{HostStart: 9000, HostEnd: 9001, ContainerStart: 9000, ContainerEnd: 9001, Protocol: "tcp"}},
		{in: "8000-8010:80", want: Binding{HostStart: 8000, HostEnd: 8010, ContainerStart: 80, ContainerEnd: 80, Protocol: "tcp"}},
		{in: "80:http", err: "not a port number"},
		{in: "70000:80", err: "out of range"},
		{in: "8080:80/xyz", err: "unknown protocol"},
		{in: "9001-9000:80", err: "reversed"},
		{in: "9000-9002:80-81", err: "different sizes"},
		{in: "::1:8080:80", err: "too many"},
	}
	for _, tt := range tests {
		got, err := ParseShortPort(tt.in)
		if tt.err != "" {
			if err == nil || !strings.Contains(err.Error(), tt.err) {
				t.Errorf("%q: error %v, want %q", tt.in, err, tt.err)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("%q: got %+v, %v want %+v", tt.in, got, err, tt.want)
		}
	}
}

func TestConflicts(t *testing.T) {
	p := func(s string) Binding {
		b, err := ParseShortPort(s)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	cases := []struct {
		a, b string
		want bool
	}{
		{"8080:80", "8080:81", true},
		{"8080:80", "8080:80/udp", false},
		{"127.0.0.1:8080:80", "127.0.0.2:8080:80", false},
		{"127.0.0.1:8080:80", "8080:80", true},
		{"0.0.0.0:8080:80", "[::1]:8080:80", true},
		{"[::1]:8080:80", "127.0.0.1:8080:80", false},
		{"9000-9005:9000-9005", "9005:80", true},
		{"9000-9005:9000-9005", "9006:80", false},
		{"80", "80", false},
		{"8000-8010:80", "8000-8010:80", false},
	}
	for _, c := range cases {
		if got := Conflicts(p(c.a), p(c.b)); got != c.want {
			t.Errorf("%s vs %s: got %v want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestPortRules(t *testing.T) {
	runCases(t, []string{"CD301", "CD302"}, []ruleCase{
		{name: "distinct ports", files: map[string]string{"compose.yaml": "services:\n  a: {image: x, ports: [\"8080:80\"]}\n  b: {image: x, ports: [\"8081:80\", \"80\", \"127.0.0.1::80\"]}\n"}},
		{name: "conflict across services", files: map[string]string{
			"compose.yaml": "services:\n  api:\n    image: x\n    ports: [\"8080:80\"]\n  web:\n    image: x\n    ports:\n      - \"8080:3000\"\n"},
			want: []string{"CD301@compose.yaml:8"}},
		{name: "conflict long syntax vs short", files: map[string]string{
			"compose.yaml": "services:\n  api:\n    image: x\n    ports:\n      - target: 80\n        published: \"5432\"\n  db:\n    image: x\n    ports: [\"127.0.0.1:5432:5432\"]\n"},
			want: []string{"CD301@compose.yaml:9"}},
		{name: "conflict introduced by override", files: map[string]string{
			"compose.yaml":          "services:\n  api: {image: x, ports: [\"3000:3000\"]}\n  web: {image: x}\n",
			"compose.override.yaml": "services:\n  web:\n    ports: [\"3000:80\"]\n"},
			want: []string{"CD301@compose.override.yaml:3"}},
		{name: "same port in base and override is merged, not a conflict", files: map[string]string{
			"compose.yaml":          "services:\n  api: {image: x, ports: [\"3000:3000\"]}\n",
			"compose.override.yaml": "services:\n  api: {ports: [\"3000:3000\"]}\n"}},
		{name: "interpolated conflict", files: map[string]string{
			"compose.yaml": "services:\n  a: {image: x, ports: [\"${PORT:-8080}:80\"]}\n  b: {image: x, ports: [\"8080:80\"]}\n"},
			want: []string{"CD301@compose.yaml:3"}},
		{name: "inactive profile ignored", files: map[string]string{
			"compose.yaml": "services:\n  a: {image: x, ports: [\"8080:80\"]}\n  b: {image: x, profiles: [dev], ports: [\"8080:80\"]}\n"}},
		{name: "active profile included", opts: optsProfiles("dev"), files: map[string]string{
			"compose.yaml": "services:\n  a: {image: x, ports: [\"8080:80\"]}\n  b: {image: x, profiles: [dev], ports: [\"8080:80\"]}\n"},
			want: []string{"CD301@compose.yaml:3"}},
		{name: "host network ignored", files: map[string]string{
			"compose.yaml": "services:\n  a: {image: x, ports: [\"8080:80\"]}\n  b: {image: x, network_mode: host, ports: [\"8080:80\"]}\n"}},
		{name: "invalid ports", files: map[string]string{
			"compose.yaml": "services:\n  a:\n    image: x\n    ports:\n      - \"80:http\"\n      - target: 80\n        protocol: icmp\n      - published: 8080\n"},
			want: []string{"CD302@compose.yaml:5", "CD302@compose.yaml:6", "CD302@compose.yaml:8"}},
		{name: "unresolved variable skipped", files: map[string]string{
			"compose.yaml": "services:\n  a: {image: x, ports: [\"${PORT}:80\"]}\n"}},
	})
}

func TestPortConflictMessages(t *testing.T) {
	f := findOne(t, map[string]string{"compose.yaml": "services:\n  api: {image: x, ports: [\"8080:80\"]}\n  web: {image: x, ports: [\"8080:3000\"]}\n"}, nil, "CD301")
	want := `host port 8080/tcp is published twice by services "api" and "web": "8080:80" at compose.yaml:2:27 and "8080:3000"`
	if f.Message != want {
		t.Errorf("message:\n got  %s\n want %s", f.Message, want)
	}
	if f.Evidence["other_service"] != "api" || f.Evidence["service"] != "web" {
		t.Errorf("evidence: %v", f.Evidence)
	}
	f = findOne(t, map[string]string{"compose.yaml": "services:\n  api: {image: x, ports: [\"${P}:80\"]}\n  web: {image: x, ports: [\"${P}:3000\"]}\n", ".env": "P=7777\n"}, nil, "CD301")
	if strings.Contains(f.Message, "7777") || !strings.Contains(f.Message, "${P}:80") {
		t.Errorf("interpolated message must show raw spec only: %s", f.Message)
	}
}
