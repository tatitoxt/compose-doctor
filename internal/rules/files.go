package rules

import (
	"fmt"
	"os"
	"strings"

	"github.com/tatitoxt/compose-doctor/internal/compose"
)

func fileRules() []Rule {
	return []Rule{
		{ID: "CD101", Name: "env-file-not-found", Severity: Error, check: checkEnvFiles,
			Summary: "A required `env_file` does not exist (paths are relative to the project directory)."},
		{ID: "CD102", Name: "build-context-not-found", Severity: Error, check: checkBuild,
			Summary: "A local `build` context directory or its Dockerfile does not exist."},
		{ID: "CD103", Name: "bind-source-not-found", Severity: Warning, check: checkBindSources,
			Summary: "A relative bind-mount source (`./…`, `../…`) does not exist."},
		{ID: "CD104", Name: "watch-path-not-found", Severity: Warning, check: checkWatchPaths,
			Summary: "A relative `develop.watch` path does not exist."},
	}
}

// stat reports whether a project-relative path exists and is a directory.
func stat(p *compose.Project, path string) (exists, isDir bool) {
	fi, err := os.Stat(p.Resolve(path))
	if err != nil {
		return false, false
	}
	return true, fi.IsDir()
}

// shown returns the path as written in the file, never the interpolated
// value, so variable values cannot leak through paths.
func shown(v compose.Value) string { return v.Raw }

func checkEnvFiles(r Rule, p *compose.Project) []Finding {
	var out []Finding
	for _, s := range p.ActiveServices() {
		for _, ef := range s.EnvFiles {
			if !ef.Required || ef.Path.Unresolved || ef.Path.Value == "" {
				continue
			}
			exists, isDir := stat(p, ef.Path.Value)
			if exists && !isDir {
				continue
			}
			msg := fmt.Sprintf("service %q: env_file %s not found", s.Name, quote(shown(ef.Path)))
			if isDir {
				msg = fmt.Sprintf("service %q: env_file %s is a directory, not a file", s.Name, quote(shown(ef.Path)))
			}
			suggestion := "Check the file name and location; env_file paths are resolved from the project directory (where the first Compose file lives). If the file is optional, use the long syntax with `required: false`."
			if !ef.Path.Interpolated {
				if ok, dir := stat(p, ef.Path.Value+".example"); ok && !dir {
					suggestion = fmt.Sprintf("A template exists: copy %s to %s and fill in local values. %s",
						quote(ef.Path.Value+".example"), quote(ef.Path.Value), suggestion)
				}
			}
			out = append(out, r.finding(ef.Path.Loc, msg, suggestion,
				map[string]string{"service": s.Name, "path": shown(ef.Path)}))
		}
	}
	return out
}

func isRemoteContext(ctx string) bool {
	return strings.Contains(ctx, "://") || strings.HasPrefix(ctx, "git@") || strings.HasPrefix(ctx, "github.com/")
}

func checkBuild(r Rule, p *compose.Project) []Finding {
	var out []Finding
	for _, s := range p.ActiveServices() {
		b := s.Build
		if b == nil || b.Context.Unresolved || isRemoteContext(b.Context.Value) {
			continue
		}
		ctx := b.Context.Value
		if ctx == "" {
			ctx = "."
		}
		exists, isDir := stat(p, ctx)
		if !exists || !isDir {
			what := "not found"
			if exists {
				what = "is not a directory"
			}
			out = append(out, r.finding(b.Context.Loc,
				fmt.Sprintf("service %q: build context %s %s", s.Name, quote(shown(b.Context)), what),
				"Check the path; build contexts are resolved from the project directory.",
				map[string]string{"service": s.Name, "context": shown(b.Context)}))
			continue
		}
		if b.Inline {
			continue
		}
		if b.Dockerfile != nil {
			if b.Dockerfile.Unresolved {
				continue
			}
			df := b.Dockerfile.Value
			path := df
			if !strings.HasPrefix(df, "/") {
				path = strings.TrimSuffix(ctx, "/") + "/" + df
			}
			if ok, dir := stat(p, path); !ok || dir {
				out = append(out, r.finding(b.Dockerfile.Loc,
					fmt.Sprintf("service %q: dockerfile %s not found in build context %s", s.Name, quote(shown(*b.Dockerfile)), quote(shown(b.Context))),
					"`dockerfile` is resolved relative to the build context, not to the Compose file.",
					map[string]string{"service": s.Name, "context": shown(b.Context), "dockerfile": shown(*b.Dockerfile)}))
			}
			continue
		}
		okD, dirD := stat(p, ctx+"/Dockerfile")
		if !okD || dirD {
			out = append(out, r.finding(b.Loc,
				fmt.Sprintf("service %q: no Dockerfile in build context %s", s.Name, quote(shown(b.Context))),
				"Docker Compose defaults to `Dockerfile` — a lone `Containerfile` is not used unless `build.dockerfile` names it. Add a Dockerfile, set `build.dockerfile`, or use `build.dockerfile_inline`.",
				map[string]string{"service": s.Name, "context": shown(b.Context)}))
		}
	}
	return out
}

func isRelativePath(src string) bool {
	return src == "." || src == ".." || strings.HasPrefix(src, "./") || strings.HasPrefix(src, "../") ||
		strings.HasPrefix(src, `.\`) || strings.HasPrefix(src, `..\`)
}

func checkBindSources(r Rule, p *compose.Project) []Finding {
	var out []Finding
	for _, s := range p.ActiveServices() {
		for _, v := range s.Volumes {
			if v.Type != "bind" || v.Source.Unresolved || !isRelativePath(v.Source.Value) {
				continue
			}
			if exists, _ := stat(p, v.Source.Value); exists {
				continue
			}
			suggestion := "Docker creates a missing short-syntax bind source as an empty, root-owned directory, which usually hides a typo. Create the path or fix the name."
			if !v.Short {
				suggestion = "With the long volume syntax Docker Compose fails with \"bind source path does not exist\". Create the path or fix the name."
			}
			src := v.Source.Raw
			if v.Short {
				src, _ = compose.SplitShortVolume(v.Source.Raw)
			}
			out = append(out, r.finding(v.Source.Loc,
				fmt.Sprintf("service %q: bind mount source %s does not exist", s.Name, quote(src)),
				suggestion,
				map[string]string{"service": s.Name, "source": src}))
		}
	}
	return out
}

func checkWatchPaths(r Rule, p *compose.Project) []Finding {
	var out []Finding
	for _, s := range p.ActiveServices() {
		for _, w := range s.Watch {
			if w.Path.Unresolved || w.Path.Value == "" || isAbsolutePath(w.Path.Value) {
				continue
			}
			if exists, _ := stat(p, w.Path.Value); exists {
				continue
			}
			out = append(out, r.finding(w.Path.Loc,
				fmt.Sprintf("service %q: develop.watch path %s not found", s.Name, quote(shown(w.Path))),
				"`docker compose watch` only syncs paths that exist; a missing path usually means a typo or a directory that has not been created yet.",
				nil))
		}
	}
	return out
}
