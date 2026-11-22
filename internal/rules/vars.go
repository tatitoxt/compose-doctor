package rules

import (
	"fmt"
	"strings"

	"github.com/tatitoxt/compose-doctor/internal/compose"
)

func variableRules() []Rule {
	return []Rule{
		{ID: "CD201", Name: "undefined-variable", Severity: Warning, check: checkUndefinedVars,
			Summary: "A variable is interpolated without a default but is not set in the shell environment or the project .env file; Compose substitutes an empty string."},
		{ID: "CD202", Name: "required-variable-unset", Severity: Error, check: checkRequiredVars,
			Summary: "A variable marked as required (`${VAR:?msg}` / `${VAR?msg}`) is not set; Docker Compose refuses to start."},
		{ID: "CD203", Name: "invalid-interpolation", Severity: Error, check: checkInterpErrors,
			Summary: "A value contains malformed interpolation syntax (for example an unclosed `${`)."},
	}
}

type varGroup struct {
	name string
	uses []compose.VarUse
}

func groupUses(p *compose.Project, keep func(compose.VarRef) bool) []varGroup {
	idx := map[string]int{}
	var groups []varGroup
	for _, u := range p.VarUses {
		if !u.Ref.Problem || !keep(u.Ref) {
			continue
		}
		i, ok := idx[u.Ref.Name]
		if !ok {
			i = len(groups)
			idx[u.Ref.Name] = i
			groups = append(groups, varGroup{name: u.Ref.Name})
		}
		groups[i].uses = append(groups[i].uses, u)
	}
	return groups
}

func (g varGroup) evidence(u compose.VarUse) map[string]string {
	ev := map[string]string{"variable": g.name, "path": u.Path}
	var others []string
	for _, o := range g.uses {
		if o.Loc == u.Loc && o.Path == u.Path {
			continue
		}
		others = append(others, o.Loc.String())
	}
	if len(others) > 0 {
		ev["other_locations"] = strings.Join(others, ", ")
	}
	return ev
}

func (g varGroup) countSuffix() string {
	if len(g.uses) == 1 {
		return ""
	}
	return fmt.Sprintf(" (referenced %d times)", len(g.uses))
}

// varSuggestion explains where a variable could come from. It only ever
// mentions names and file paths.
func varSuggestion(p *compose.Project, name string) string {
	var hints []string
	for _, s := range p.ActiveServices() {
		for _, ef := range s.EnvFiles {
			if ef.Path.Unresolved {
				continue
			}
			for _, n := range p.EnvFileNames(ef.Path.Value) {
				if n == name {
					hints = append(hints, fmt.Sprintf("%s is declared in env_file %s of service %q, but env_file only sets the container environment; Compose interpolation reads only the shell environment and the project .env file.", name, quote(ef.Path.Raw), s.Name))
				}
			}
		}
	}
	if len(hints) > 1 {
		hints = hints[:1]
	}
	switch {
	case p.InEnvExample(name) && p.DotenvFile == "":
		hints = append(hints, fmt.Sprintf("%s is listed in .env.example: copy .env.example to .env and set a local value.", name))
	case p.InEnvExample(name):
		hints = append(hints, fmt.Sprintf("%s is listed in .env.example but missing from %s: add it there.", name, p.DotenvFile))
	default:
		hints = append(hints, fmt.Sprintf("Set %s in the project .env file or the shell environment.", name))
	}
	return strings.Join(hints, " ")
}

func checkUndefinedVars(r Rule, p *compose.Project) []Finding {
	var out []Finding
	for _, g := range groupUses(p, func(ref compose.VarRef) bool { return !ref.Required() }) {
		for _, u := range g.uses {
			out = append(out, r.finding(u.Loc,
				fmt.Sprintf("variable %s is not set and will be replaced by an empty string%s", g.name, g.countSuffix()),
				varSuggestion(p, g.name)+fmt.Sprintf(" If empty is intended, write ${%s:-} to make it explicit.", g.name),
				g.evidence(u)))
		}
	}
	return out
}

func checkRequiredVars(r Rule, p *compose.Project) []Finding {
	var out []Finding
	for _, g := range groupUses(p, compose.VarRef.Required) {
		state := "is not set"
		if p.LookupEnv(g.name) {
			state = "is set but empty"
		}
		for _, u := range g.uses {
			out = append(out, r.finding(u.Loc,
				fmt.Sprintf("required variable %s %s; docker compose will refuse to start%s", g.name, state, g.countSuffix()),
				varSuggestion(p, g.name),
				g.evidence(u)))
		}
	}
	return out
}

func checkInterpErrors(r Rule, p *compose.Project) []Finding {
	var out []Finding
	for _, e := range p.InterpErrors {
		out = append(out, r.finding(e.Loc,
			"invalid interpolation: "+e.Msg,
			"Use ${NAME}, ${NAME:-default} or ${NAME:?error}; write $$ for a literal dollar sign.",
			map[string]string{"path": e.Path}))
	}
	return out
}
