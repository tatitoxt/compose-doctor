package rules

import (
	"fmt"

	"github.com/tatitoxt/compose-doctor/internal/compose"
)

func dependencyRules() []Rule {
	return []Rule{
		{ID: "CD401", Name: "depends-on-undefined-service", Severity: Error, check: checkUndefinedDeps,
			Summary: "`depends_on` names a service that is not defined in any loaded Compose file."},
		{ID: "CD402", Name: "depends-on-without-readiness", Severity: Info, check: checkReadiness,
			Summary: "`depends_on` only waits for the dependency to start, not to be ready. Advisory: suggests `condition: service_healthy` with a healthcheck."},
		{ID: "CD403", Name: "healthy-condition-without-healthcheck", Severity: Warning, check: checkHealthyWithoutCheck,
			Summary: "`condition: service_healthy` targets a service with no healthcheck in Compose; it fails unless the image defines HEALTHCHECK."},
	}
}

func hasHealthcheck(s *compose.Service) bool { return s.Healthcheck != nil && !s.Healthcheck.Disabled }

func checkUndefinedDeps(r Rule, p *compose.Project) []Finding {
	var out []Finding
	for _, s := range p.ActiveServices() {
		for _, d := range s.DependsOn {
			if _, ok := p.Services[d.Service]; ok || !d.Required {
				continue
			}
			out = append(out, r.finding(d.Loc,
				fmt.Sprintf("service %q depends on undefined service %q", s.Name, d.Service),
				"Fix the service name, or load the file that defines it with an extra -f. Use `required: false` if the dependency is optional.",
				map[string]string{"service": s.Name, "dependency": d.Service}))
		}
	}
	return out
}

func checkReadiness(r Rule, p *compose.Project) []Finding {
	var out []Finding
	for _, s := range p.ActiveServices() {
		for _, d := range s.DependsOn {
			dep, ok := p.Services[d.Service]
			if !ok || !p.IsActive(dep) || (d.Condition != "" && d.Condition != "service_started") {
				continue
			}
			suggestion := fmt.Sprintf("Add a healthcheck to %q and use the long syntax `depends_on: {%s: {condition: service_healthy}}`, or make %q retry its connection on start-up.", d.Service, d.Service, s.Name)
			if hasHealthcheck(dep) {
				suggestion = fmt.Sprintf("%q already has a healthcheck: use `depends_on: {%s: {condition: service_healthy}}`.", d.Service, d.Service)
			}
			out = append(out, r.finding(d.Loc,
				fmt.Sprintf("service %q waits only for %q to start, not to be ready", s.Name, d.Service),
				suggestion,
				map[string]string{"service": s.Name, "dependency": d.Service, "condition": conditionOrDefault(d.Condition)}))
		}
	}
	return out
}

func conditionOrDefault(c string) string {
	if c == "" {
		return "service_started"
	}
	return c
}

func checkHealthyWithoutCheck(r Rule, p *compose.Project) []Finding {
	var out []Finding
	for _, s := range p.ActiveServices() {
		for _, d := range s.DependsOn {
			dep, ok := p.Services[d.Service]
			if !ok || d.Condition != "service_healthy" || hasHealthcheck(dep) {
				continue
			}
			state := "has no healthcheck"
			if dep.Healthcheck != nil {
				state = "has its healthcheck disabled"
			}
			out = append(out, r.finding(d.Loc,
				fmt.Sprintf("service %q waits for %q to be healthy, but %q %s in Compose", s.Name, d.Service, d.Service, state),
				fmt.Sprintf("Add a `healthcheck:` to %q. This is fine only if its image defines a Dockerfile HEALTHCHECK, which Compose Doctor cannot see offline.", d.Service),
				map[string]string{"service": s.Name, "dependency": d.Service}))
		}
	}
	return out
}
