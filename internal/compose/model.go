package compose

import (
	"gopkg.in/yaml.v3"
	"sort"
	"strings"
)

// Value is an interpolated scalar together with its raw text and location.
type Value struct {
	Raw   string
	Value string
	Loc   Location
	// Interpolated reports whether Raw contained a variable reference.
	Interpolated bool
	// Unresolved reports whether a reference could not be resolved or the
	// interpolation syntax was invalid; rules should not trust Value then.
	Unresolved bool
}

// Service is the normalised subset of a Compose service used by rules.
type Service struct {
	Name        string
	Loc         Location
	Image       *Value
	Build       *Build
	Extends     bool
	EnvFiles    []EnvFileRef
	Ports       []Port
	Volumes     []Volume
	Networks    []NetworkRef
	DependsOn   []Dependency
	Healthcheck *Healthcheck
	Profiles    []string
	NetworkMode *Value
	// Restart is the service's restart policy (e.g. "unless-stopped").
	Restart *Value
	// ResourceLimits reports deploy.resources.limits being set; LimitsLoc
	// points at the `deploy` key that carried it.
	ResourceLimits bool
	LimitsLoc      Location
	// Watch lists develop.watch rules for `docker compose watch`.
	Watch []WatchRule
}

// WatchRule is one develop.watch entry: a host path watched for changes and
// the action taken ("sync", "rebuild", "sync+restart", "sync+exec", "ignore").
type WatchRule struct {
	Loc    Location
	Path   Value
	Action string
}

// Build is the build section of a service.
type Build struct {
	Loc        Location
	Context    Value
	Dockerfile *Value
	Inline     bool
}

// EnvFileRef is one env_file entry.
type EnvFileRef struct {
	Path     Value
	Required bool
}

// Port is one ports entry, in short ("8080:80") or long syntax.
type Port struct {
	Loc   Location
	Short *Value
	// Long syntax fields (nil when absent).
	Target, Published, HostIP, Protocol *Value
}

func (p Port) key() string {
	if p.Short != nil {
		return "s:" + p.Short.Raw
	}
	parts := []string{"l"}
	for _, v := range []*Value{p.HostIP, p.Published, p.Target, p.Protocol} {
		if v == nil {
			parts = append(parts, "")
		} else {
			parts = append(parts, v.Raw)
		}
	}
	return strings.Join(parts, "|")
}

// Volume is one service volumes entry.
type Volume struct {
	Loc Location
	// Type is "bind", "volume", "tmpfs", "npipe", "cluster" or "image".
	Type   string
	Source Value
	Target string
	Short  bool
}

// NetworkRef is a network a service attaches to.
type NetworkRef struct {
	Name string
	// Shown is Name, or the raw text when the name was interpolated.
	Shown string
	Loc   Location
}

// Dependency is one depends_on entry.
type Dependency struct {
	Service   string
	Shown     string // Service, or the raw text when interpolated
	Condition string // empty for short syntax
	Required  bool
	Short     bool
	Loc       Location
}

// Healthcheck is the healthcheck section of a service.
type Healthcheck struct {
	Loc      Location
	Disabled bool
	// disableKey and testNone record the two ways of disabling a
	// healthcheck so field-wise override merges can recompute Disabled.
	disableKey, testNone bool
}

// VarUse is a variable reference found in a Compose file.
type VarUse struct {
	Ref  VarRef
	Loc  Location
	Path string // YAML path such as services.web.environment.DB_URL
}

// InterpProblem is malformed interpolation syntax in a scalar.
type InterpProblem struct {
	Loc  Location
	Path string
	Msg  string
}

// DuplicateKey is a mapping key defined twice in the same mapping.
type DuplicateKey struct {
	Key   string
	Path  string
	Loc   Location
	First Location
}

// Project is a loaded, merged Compose project.
type Project struct {
	// Dir is the absolute project directory.
	Dir string
	// Files are the loaded Compose files, relative to Dir.
	Files []string
	// DotenvFile is the .env file used for interpolation, relative to Dir ("" if none).
	DotenvFile string
	// ActiveProfiles are the enabled profiles.
	ActiveProfiles []string

	Services map[string]*Service
	Volumes  map[string]Location
	Networks map[string]Location

	VersionKeys  []Location
	VarUses      []VarUse
	InterpErrors []InterpProblem
	DupKeys      []DuplicateKey

	env        map[string]string
	envExample map[string]bool

	// Load bookkeeping: parsed YAML roots by abs path (used by extends)
	// and display-path -> abs-path for the file being merged.
	parsed  map[string]*yaml.Node
	merged  map[string]bool
	scanned map[string]bool
	absOf   map[string]string
}

// LookupEnv reports whether an interpolation variable is set. The value is
// deliberately not returned to callers outside this package.
func (p *Project) LookupEnv(name string) bool {
	_, ok := p.env[name]
	return ok
}

// InEnvExample reports whether name is declared in the project .env.example.
func (p *Project) InEnvExample(name string) bool { return p.envExample[name] }

// HasEnvExample reports whether the project has a .env.example file.
func (p *Project) HasEnvExample() bool { return p.envExample != nil }

// ServiceNames returns all service names, sorted.
func (p *Project) ServiceNames() []string {
	names := make([]string, 0, len(p.Services))
	for n := range p.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ActiveServices returns services enabled by the active profiles, sorted by name.
func (p *Project) ActiveServices() []*Service {
	var out []*Service
	for _, n := range p.ServiceNames() {
		if s := p.Services[n]; p.IsActive(s) {
			out = append(out, s)
		}
	}
	return out
}

// IsActive reports whether a service is enabled by the active profiles.
func (p *Project) IsActive(s *Service) bool {
	if len(s.Profiles) == 0 {
		return true
	}
	for _, a := range p.ActiveProfiles {
		if a == "*" {
			return true
		}
		for _, sp := range s.Profiles {
			if sp == a {
				return true
			}
		}
	}
	return false
}
