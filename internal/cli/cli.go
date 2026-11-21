// Package cli implements the compose-doctor command line.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/tatitoxt/compose-doctor/internal/compose"
	"github.com/tatitoxt/compose-doctor/internal/config"
	"github.com/tatitoxt/compose-doctor/internal/report"
	"github.com/tatitoxt/compose-doctor/internal/rules"
)

// Exit codes (see docs/adr/0003-severity-and-exit-codes.md).
const (
	ExitOK           = 0
	ExitFindings     = 1
	ExitUsage        = 2
	ExitInvalidInput = 3
)

// Version is set at build time with -ldflags "-X .../internal/cli.Version=...".
var Version = "dev"

const usage = `compose-doctor diagnoses Docker Compose projects offline.

Usage:
  compose-doctor check [flags] [path]   analyse a project (run "compose-doctor check -h")
  compose-doctor rules [--format json]  list the rules
  compose-doctor version                print the version

It never runs containers, invokes Docker or a shell, opens network
connections or modifies files.
`

// Run executes the CLI and returns the process exit code. environ is the
// process environment (KEY=VALUE).
func Run(args []string, stdout, stderr io.Writer, environ []string) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	switch args[0] {
	case "check":
		return runCheck(args[1:], stdout, stderr, environ)
	case "rules":
		return runRules(args[1:], stdout, stderr)
	case "version", "--version":
		fmt.Fprintf(stdout, "compose-doctor %s\n", Version)
		return ExitOK
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return ExitOK
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
		return ExitUsage
	}
}

func runCheck(args []string, stdout, stderr io.Writer, environ []string) int {
	opts, err := config.ParseCheck(args, stderr)
	if errors.Is(err, config.ErrHelp) {
		return ExitOK
	}
	if err != nil {
		fmt.Fprintf(stderr, "compose-doctor: %v\n", err)
		return ExitUsage
	}
	if err := opts.ApplyRulesFile(); err != nil {
		fmt.Fprintf(stderr, "compose-doctor: %v\n", err)
		return ExitUsage
	}
	if opts.IgnoreOSEnv {
		environ = nil
	}
	project, err := compose.Load(compose.Options{
		Path: opts.Path, Files: opts.Files, EnvFile: opts.EnvFile, Environ: environ, Profiles: opts.Profiles,
	})
	if err != nil {
		var le *compose.LoadError
		if !errors.As(err, &le) {
			le = &compose.LoadError{Loc: compose.Location{File: opts.Path}, Msg: err.Error()}
		}
		findings := []rules.Finding{rules.FromLoadError(le)}
		if werr := write(stdout, opts.Format, report.New(Version, nil, opts.Profiles, findings)); werr != nil {
			fmt.Fprintf(stderr, "compose-doctor: %v\n", werr)
		}
		return ExitInvalidInput
	}
	findings := rules.Run(project, opts.Disabled)
	for i := range findings {
		if sev, ok := opts.Severities[findings[i].RuleID]; ok {
			findings[i].Severity = sev
		}
	}
	if err := write(stdout, opts.Format, report.New(Version, project.Files, project.ActiveProfiles, findings)); err != nil {
		fmt.Fprintf(stderr, "compose-doctor: %v\n", err)
		return ExitInvalidInput
	}
	if opts.Strict {
		for _, f := range findings {
			if f.Severity >= opts.FailOn {
				return ExitFindings
			}
		}
	}
	return ExitOK
}

func write(w io.Writer, format string, r report.Report) error {
	switch format {
	case "json":
		return report.JSON(w, r)
	case "sarif":
		return report.SARIF(w, r)
	}
	return report.Human(w, r)
}

func runRules(args []string, stdout, stderr io.Writer) int {
	format := "human"
	switch {
	case len(args) == 0:
	case len(args) == 1 && (args[0] == "--format=json" || args[0] == "-format=json"):
		format = "json"
	case len(args) == 2 && (args[0] == "--format" || args[0] == "-format") && (args[1] == "json" || args[1] == "human"):
		format = args[1]
	case len(args) == 1 && (args[0] == "--format=human" || args[0] == "-format=human"):
	default:
		fmt.Fprintln(stderr, "usage: compose-doctor rules [--format human|json]")
		return ExitUsage
	}
	all := rules.All()
	if format == "json" {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		_ = enc.Encode(all)
		return ExitOK
	}
	for _, r := range all {
		fmt.Fprintf(stdout, "%s  %-7s  %-38s %s\n", r.ID, r.Severity, r.Name, r.Summary)
	}
	return ExitOK
}
