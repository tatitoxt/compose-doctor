package rules

import (
	"fmt"
	"strings"

	"github.com/tatitoxt/compose-doctor/internal/compose"
)

func opsRules() []Rule {
	return []Rule{
		{ID: "CD601", Name: "restart-policy-missing", Severity: Info, check: checkRestartPolicy,
			Summary: "A service has no `restart` policy; containers default to `no` and stay down after a crash or reboot."},
		{ID: "CD602", Name: "resource-limits-missing", Severity: Info, check: checkResourceLimits,
			Summary: "A service sets no `deploy.resources.limits`; a runaway container can starve the host."},
		{ID: "CD603", Name: "latest-tag", Severity: Warning, check: checkLatestTag,
			Summary: "An image uses the `latest` tag or no tag; builds are not reproducible and pulls are surprising."},
	}
}

func checkRestartPolicy(r Rule, p *compose.Project) []Finding {
	var out []Finding
	for _, s := range p.ActiveServices() {
		if s.Restart != nil {
			continue
		}
		out = append(out, r.finding(s.Loc,
			fmt.Sprintf("service %q has no restart policy", s.Name),
			"Add `restart: unless-stopped` (or `always`) so the container survives crashes and host reboots.",
			map[string]string{"service": s.Name}))
	}
	return out
}

func checkResourceLimits(r Rule, p *compose.Project) []Finding {
	var out []Finding
	for _, s := range p.ActiveServices() {
		if s.ResourceLimits {
			continue
		}
		out = append(out, r.finding(s.Loc,
			fmt.Sprintf("service %q sets no resource limits", s.Name),
			"Add `deploy.resources.limits` (cpus/memory) so one container cannot starve the host.",
			map[string]string{"service": s.Name}))
	}
	return out
}

func checkLatestTag(r Rule, p *compose.Project) []Finding {
	var out []Finding
	for _, s := range p.ActiveServices() {
		if s.Image == nil {
			continue
		}
		img := s.Image.Value
		// Strip a registry host (first segment containing . or : or being
		// localhost), then look at the tag after the last ':' on the name.
		name := img[strings.LastIndex(img, "/")+1:]
		tag := ""
		if i := strings.LastIndex(name, ":"); i >= 0 {
			tag = name[i+1:]
		}
		if tag == "" || tag == "latest" {
			out = append(out, r.finding(s.Image.Loc,
				fmt.Sprintf("service %q uses image %q without a pinned tag", s.Name, img),
				"Pin a version tag (e.g. `nginx:1.25`) or a digest so builds are reproducible.",
				map[string]string{"service": s.Name, "image": img}))
		}
	}
	return out
}
