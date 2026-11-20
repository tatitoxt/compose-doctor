package rules

import (
	"fmt"

	"github.com/tatitoxt/compose-doctor/internal/compose"
)

func referenceRules() []Rule {
	return []Rule{
		{ID: "CD501", Name: "undefined-volume", Severity: Error, check: checkUndefinedVolumes,
			Summary: "A service mounts a named volume that is not declared under top-level `volumes`."},
		{ID: "CD502", Name: "undefined-network", Severity: Error, check: checkUndefinedNetworks,
			Summary: "A service joins a network that is not declared under top-level `networks` (`default` is implicit)."},
	}
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
