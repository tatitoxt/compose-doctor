// Package config parses and validates the options of the check command.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/tatitoxt/compose-doctor/internal/rules"
)

// Check holds the options of `compose-doctor check`.
type Check struct {
	Path        string
	Files       []string
	EnvFile     string
	Profiles    []string
	Format      string
	Strict      bool
	FailOn      rules.Severity
	Disabled    map[string]bool
	IgnoreOSEnv bool
}

type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error {
	*l = append(*l, v)
	return nil
}

// ErrHelp is returned when -h/--help was requested.
var ErrHelp = flag.ErrHelp

// ParseCheck parses check arguments. Flags may appear before or after the path.
func ParseCheck(args []string, stderr io.Writer) (Check, error) {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var files, profiles, disable listFlag
	c := Check{}
	fs.Var(&files, "f", "Compose file (repeatable; later files override earlier ones)")
	fs.Var(&files, "file", "alias of -f")
	fs.Var(&profiles, "profile", "enable a profile (repeatable; \"*\" enables all)")
	fs.Var(&disable, "disable", "comma-separated rule IDs to skip (repeatable)")
	fs.StringVar(&c.EnvFile, "env-file", "", "env file used for interpolation instead of <project>/.env")
	fs.StringVar(&c.Format, "format", "human", "output format: human or json")
	fs.BoolVar(&c.Strict, "strict", false, "exit 1 when a finding at or above --fail-on is reported")
	failOn := fs.String("fail-on", "warning", "minimum severity that fails --strict: info, warning or error")
	fs.BoolVar(&c.IgnoreOSEnv, "ignore-os-env", false, "do not read variables from the process environment")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: compose-doctor check [flags] [path]")
		fmt.Fprintln(stderr, "\nDiagnoses a Docker Compose project offline. path is a directory (default \".\") or a Compose file.")
		fmt.Fprintln(stderr, "\nFlags:")
		fs.PrintDefaults()
	}

	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return c, err
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
	if len(positional) > 1 {
		return c, fmt.Errorf("expected at most one path, got %d", len(positional))
	}
	c.Path = "."
	if len(positional) == 1 {
		c.Path = positional[0]
	}
	c.Files, c.Profiles = files, profiles
	if c.Format != "human" && c.Format != "json" {
		return c, fmt.Errorf("invalid --format %q (want human or json)", c.Format)
	}
	sev, err := rules.ParseSeverity(*failOn)
	if err != nil {
		return c, fmt.Errorf("invalid --fail-on: %w", err)
	}
	c.FailOn = sev
	c.Disabled = map[string]bool{}
	for _, entry := range disable {
		for _, id := range strings.Split(entry, ",") {
			id = strings.ToUpper(strings.TrimSpace(id))
			if id == "" {
				continue
			}
			if _, ok := rules.Lookup(id); !ok {
				return c, fmt.Errorf("unknown rule %q in --disable (see `compose-doctor rules`)", id)
			}
			if id == rules.ParseError.ID {
				return c, errors.New("CD001 cannot be disabled: it reports input that cannot be analysed")
			}
			c.Disabled[id] = true
		}
	}
	return c, nil
}
