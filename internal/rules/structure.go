package rules

import (
	"fmt"

	"github.com/tatitoxt/compose-doctor/internal/compose"
)

func structureRules() []Rule {
	return []Rule{
		{ID: "CD002", Name: "service-without-image-or-build", Severity: Error, check: checkMissingImage,
			Summary: "A service defines neither `image` nor `build` (and does not use `extends`)."},
		{ID: "CD003", Name: "obsolete-version-key", Severity: Info, check: checkVersionKey,
			Summary: "The top-level `version` key is obsolete; Docker Compose v2 ignores it."},
		{ID: "CD004", Name: "duplicate-key", Severity: Error, check: checkDuplicateKeys,
			Summary: "A key is defined twice in the same mapping; Docker Compose rejects the file."},
	}
}

func checkMissingImage(r Rule, p *compose.Project) []Finding {
	var out []Finding
	for _, s := range p.ActiveServices() {
		if s.Image != nil || s.Build != nil || s.Extends {
			continue
		}
		out = append(out, r.finding(s.Loc,
			fmt.Sprintf("service %q has neither an image nor a build context", s.Name),
			"Add `image: <name>` to pull an image or `build: <path>` to build one.",
			map[string]string{"service": s.Name}))
	}
	return out
}

func checkVersionKey(r Rule, p *compose.Project) []Finding {
	var out []Finding
	for _, l := range p.VersionKeys {
		out = append(out, r.finding(l,
			"the top-level `version` key is obsolete and ignored by Docker Compose v2",
			"Remove the `version:` line; the Compose Specification does not use it.", nil))
	}
	return out
}

func checkDuplicateKeys(r Rule, p *compose.Project) []Finding {
	var out []Finding
	for _, d := range p.DupKeys {
		out = append(out, r.finding(d.Loc,
			fmt.Sprintf("key %q is defined twice in the same mapping (first at %s)", d.Key, d.First),
			"Keep a single definition; merge the two blocks if both are needed.",
			map[string]string{"key": d.Key, "path": d.Path, "first_location": d.First.String()}))
	}
	return out
}
