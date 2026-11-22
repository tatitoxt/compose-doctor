package rules

import (
	"fmt"
	"strings"

	"github.com/tatitoxt/compose-doctor/internal/compose"
)

func referenceRules() []Rule {
	return []Rule{
		{ID: "CD501", Name: "undefined-volume", Severity: Error, check: checkUndefinedVolumes,
			Summary: "A service mounts a named volume that is not declared under top-level `volumes`."},
		{ID: "CD502", Name: "undefined-network", Severity: Error, check: checkUndefinedNetworks,
			Summary: "A service joins a network that is not declared under top-level `networks` (`default` is implicit)."},
		{ID: "CD503", Name: "absolute-bind-mount", Severity: Warning, check: checkAbsoluteBindMounts,
			Summary: "A bind mount uses an absolute host path, which is not portable across machines and CI."},
	}
}

func checkAbsoluteBindMounts(r Rule, p *compose.Project) []Finding {
	var out []Finding
	for _, s := range p.ActiveServices() {
		for _, v := range s.Volumes {
			if v.Type != "bind" || v.Source.Unresolved {
				continue
			}
			src := v.Source.Value
			if !isAbsolutePath(src) {
				continue
			}
			out = append(out, r.finding(v.Source.Loc,
				fmt.Sprintf("service %q bind-mounts absolute host path %q", s.Name, src),
				"Use a project-relative path (./dir), a named volume, or an env var (e.g. ${DATA_DIR}) so the project works on other machines.",
				map[string]string{"service": s.Name, "source": src}))
		}
	}
	return out
}

// isAbsolutePath covers POSIX roots, Windows drive letters and UNC shares.
func isAbsolutePath(p string) bool {
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\\`) {
		return true
	}
	return len(p) > 2 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') &&
		(p[0] >= 'a' && p[0] <= 'z' || p[0] >= 'A' && p[0] <= 'Z')
}

func checkUndefinedVolumes(r Rule, p *compose.Project) []Finding {
	var out []Finding
	for _, s := range p.ActiveServices() {
		for _, v := range s.Volumes {
			if v.Type != "volume" || v.Source.Value == "" || v.Source.Unresolved {
				continue
			}
			if _, ok := p.Volumes[v.Source.Value]; ok {
				continue
			}
			name := v.Source.Value
			if v.Source.Interpolated {
				name = v.Source.Raw
				if v.Short {
					name, _ = compose.SplitShortVolume(v.Source.Raw)
				}
			}
			out = append(out, r.finding(v.Source.Loc,
				fmt.Sprintf("service %q uses named volume %q, which is not declared under top-level volumes", s.Name, name),
				fmt.Sprintf("Declare it (`volumes: {%s: {}}`), or use \"./%s\" if you meant a bind mount of a local directory.", name, name),
				map[string]string{"service": s.Name, "volume": name}))
		}
	}
	return out
}

func checkUndefinedNetworks(r Rule, p *compose.Project) []Finding {
	var out []Finding
	for _, s := range p.ActiveServices() {
		if s.NetworkMode != nil {
			continue
		}
		for _, n := range s.Networks {
			if n.Name == "default" {
				continue
			}
			if _, ok := p.Networks[n.Name]; ok {
				continue
			}
			out = append(out, r.finding(n.Loc,
				fmt.Sprintf("service %q joins network %q, which is not declared under top-level networks", s.Name, n.Shown),
				fmt.Sprintf("Declare it (`networks: {%s: {}}`), or mark it `external: true` if it is created outside this project.", n.Shown),
				map[string]string{"service": s.Name, "network": n.Shown}))
		}
	}
	return out
}
