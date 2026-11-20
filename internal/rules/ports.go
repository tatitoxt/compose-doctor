package rules

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tatitoxt/compose-doctor/internal/compose"
)

func portRules() []Rule {
	return []Rule{
		{ID: "CD301", Name: "host-port-conflict", Severity: Error, check: checkPortConflicts,
			Summary: "Two port mappings publish the same host IP, port and protocol. Ephemeral ports and host-port ranges mapped to a single container port are never reported."},
		{ID: "CD302", Name: "invalid-port", Severity: Error, check: checkInvalidPorts,
			Summary: "A `ports` entry cannot be parsed (bad number, range or protocol)."},
	}
}

// Binding is a parsed port mapping.
type Binding struct {
	HostIP string
	// HostStart/HostEnd are 0 when the host port is ephemeral.
	HostStart, HostEnd           int
	ContainerStart, ContainerEnd int
	Protocol                     string
}

// Ephemeral reports whether Docker picks the host port at start-up (no
// fixed host port, or a host range with a single container port).
func (b Binding) Ephemeral() bool {
	if b.HostStart == 0 {
		return true
	}
	return b.HostEnd > b.HostStart && b.ContainerStart == b.ContainerEnd
}

func parseRange(s string) (int, int, error) {
	lo, hi, isRange := strings.Cut(s, "-")
	a, err := parsePortNum(lo)
	if err != nil {
		return 0, 0, err
	}
	if !isRange {
		return a, a, nil
	}
	b, err := parsePortNum(hi)
	if err != nil {
		return 0, 0, err
	}
	if b < a {
		return 0, 0, fmt.Errorf("range %q is reversed", s)
	}
	return a, b, nil
}

func parsePortNum(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("%q is not a port number", s)
	}
	if n < 1 || n > 65535 {
		return 0, fmt.Errorf("port %d is out of range 1-65535", n)
	}
	return n, nil
}

func parseProtocol(s string) (string, error) {
	switch p := strings.ToLower(s); p {
	case "", "tcp":
		return "tcp", nil
	case "udp", "sctp":
		return p, nil
	default:
		return "", fmt.Errorf("unknown protocol %q (want tcp, udp or sctp)", s)
	}
}

// ParseShortPort parses "[[IP:]HOST:]CONTAINER[/PROTOCOL]", including
// bracketed IPv6 addresses ("[::1]:8080:80") and ranges ("8000-8001:80-81").
func ParseShortPort(spec string) (Binding, error) {
	var b Binding
	s := strings.TrimSpace(spec)
	if i := strings.LastIndex(s, "/"); i >= 0 {
		proto, err := parseProtocol(s[i+1:])
		if err != nil {
			return b, err
		}
		b.Protocol, s = proto, s[:i]
	} else {
		b.Protocol = "tcp"
	}
	if strings.HasPrefix(s, "[") {
		end := strings.Index(s, "]:")
		if end < 0 {
			return b, fmt.Errorf("unterminated IPv6 address in %q", spec)
		}
		b.HostIP, s = s[1:end], s[end+2:]
		if !strings.Contains(s, ":") {
			s = ":" + s
		}
	}
	parts := strings.Split(s, ":")
	var host, container string
	switch len(parts) {
	case 1:
		container = parts[0]
	case 2:
		host, container = parts[0], parts[1]
	case 3:
		if b.HostIP != "" {
			return b, fmt.Errorf("too many \":\" in %q", spec)
		}
		b.HostIP, host, container = parts[0], parts[1], parts[2]
	default:
		return b, fmt.Errorf("too many \":\" in %q (wrap IPv6 addresses in brackets)", spec)
	}
	return finishBinding(b, host, container)
}

func finishBinding(b Binding, host, container string) (Binding, error) {
	var err error
	if b.ContainerStart, b.ContainerEnd, err = parseRange(container); err != nil {
		return b, err
	}
	if host != "" {
		if b.HostStart, b.HostEnd, err = parseRange(host); err != nil {
			return b, err
		}
		hostLen, ctrLen := b.HostEnd-b.HostStart, b.ContainerEnd-b.ContainerStart
		if ctrLen > 0 && hostLen != ctrLen {
			return b, fmt.Errorf("host range and container range have different sizes")
		}
	}
	return b, nil
}

func parseLongPort(p compose.Port) (Binding, error) {
	b := Binding{Protocol: "tcp"}
	if p.Target == nil {
		return b, fmt.Errorf("long syntax requires `target`")
	}
	if p.Protocol != nil {
		proto, err := parseProtocol(p.Protocol.Value)
		if err != nil {
			return b, err
		}
		b.Protocol = proto
	}
	if p.HostIP != nil {
		b.HostIP = p.HostIP.Value
	}
	host := ""
	if p.Published != nil {
		host = p.Published.Value
	}
	return finishBinding(b, host, p.Target.Value)
}

type portEntry struct {
	svc          *compose.Service
	port         compose.Port
	binding      Binding
	raw          string
	interpolated bool
}

func (e portEntry) unresolved() bool {
	for _, v := range []*compose.Value{e.port.Short, e.port.Target, e.port.Published, e.port.HostIP, e.port.Protocol} {
		if v != nil && v.Unresolved {
			return true
		}
	}
	return false
}

func describePort(p compose.Port) (raw string, interpolated bool) {
	if p.Short != nil {
		return p.Short.Raw, p.Short.Interpolated
	}
	var parts []string
	for _, f := range []struct {
		name string
		v    *compose.Value
	}{{"host_ip", p.HostIP}, {"published", p.Published}, {"target", p.Target}, {"protocol", p.Protocol}} {
		if f.v != nil {
			parts = append(parts, f.name+"="+f.v.Raw)
			interpolated = interpolated || f.v.Interpolated
		}
	}
	return strings.Join(parts, " "), interpolated
}

// collectPorts parses all port entries of active services that do not use
// host networking. Unresolved entries are skipped (CD201/CD202 cover them).
func collectPorts(p *compose.Project) (valid []portEntry, invalid []portEntry, errs []error) {
	for _, s := range p.ActiveServices() {
		if s.NetworkMode != nil && s.NetworkMode.Value == "host" {
			continue
		}
		for _, port := range s.Ports {
			raw, interp := describePort(port)
			e := portEntry{svc: s, port: port, raw: raw, interpolated: interp}
			if e.unresolved() {
				continue
			}
			var err error
			if port.Short != nil {
				e.binding, err = ParseShortPort(port.Short.Value)
			} else {
				e.binding, err = parseLongPort(port)
			}
			if err != nil {
				invalid = append(invalid, e)
				errs = append(errs, err)
				continue
			}
			valid = append(valid, e)
		}
	}
	return valid, invalid, errs
}

func isWildcard(ip string) bool { return ip == "" || ip == "0.0.0.0" || ip == "::" }

// Conflicts reports whether two bindings would compete for the same host socket.
func Conflicts(a, b Binding) bool {
	if a.Ephemeral() || b.Ephemeral() || a.Protocol != b.Protocol {
		return false
	}
	if a.HostStart > b.HostEnd || b.HostStart > a.HostEnd {
		return false
	}
	return a.HostIP == b.HostIP || isWildcard(a.HostIP) || isWildcard(b.HostIP)
}

func checkPortConflicts(r Rule, p *compose.Project) []Finding {
	valid, _, _ := collectPorts(p)
	var out []Finding
	for i, cur := range valid {
		for _, prev := range valid[:i] {
			if !Conflicts(prev.binding, cur.binding) {
				continue
			}
			who := fmt.Sprintf("services %q and %q", prev.svc.Name, cur.svc.Name)
			if prev.svc == cur.svc {
				who = fmt.Sprintf("service %q", cur.svc.Name)
			}
			var msg string
			if prev.interpolated || cur.interpolated {
				msg = fmt.Sprintf("%s publish the same host port (%s): %q at %s and %q", who, cur.binding.Protocol, prev.raw, prev.port.Loc, cur.raw)
			} else {
				msg = fmt.Sprintf("host port %s/%s is published twice by %s: %q at %s and %q", hostPortText(cur.binding), cur.binding.Protocol, who, prev.raw, prev.port.Loc, cur.raw)
			}
			out = append(out, r.finding(cur.port.Loc, msg,
				"Only one mapping can bind a host port. Change one published port, bind them to different host IPs, or omit the host port to let Docker pick one.",
				map[string]string{
					"service": cur.svc.Name, "port": cur.raw,
					"other_service": prev.svc.Name, "other_port": prev.raw, "other_location": prev.port.Loc.String(),
				}))
			break
		}
	}
	return out
}

func hostPortText(b Binding) string {
	if b.HostEnd > b.HostStart {
		return fmt.Sprintf("%d-%d", b.HostStart, b.HostEnd)
	}
	return strconv.Itoa(b.HostStart)
}

func checkInvalidPorts(r Rule, p *compose.Project) []Finding {
	_, invalid, errs := collectPorts(p)
	var out []Finding
	for i, e := range invalid {
		msg := fmt.Sprintf("service %q: invalid port mapping %q: %v", e.svc.Name, e.raw, errs[i])
		if e.interpolated {
			msg = fmt.Sprintf("service %q: invalid port mapping %q after interpolation", e.svc.Name, e.raw)
		}
		out = append(out, r.finding(e.port.Loc, msg,
			"Use [HOST_IP:][HOST_PORT:]CONTAINER_PORT[/PROTOCOL], for example \"127.0.0.1:8080:80/tcp\". Quote port mappings in YAML.",
			map[string]string{"service": e.svc.Name, "port": e.raw}))
	}
	return out
}
