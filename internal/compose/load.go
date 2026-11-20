package compose

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultFiles are the file names searched in the project directory, in
// order of preference (same order as Docker Compose).
var DefaultFiles = []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"}

// Options controls how a project is loaded.
type Options struct {
	// Path is the project directory or a Compose file. Defaults to ".".
	Path string
	// Files are explicit Compose files (like docker compose -f), relative
	// to Path when Path is a directory. Disables override auto-discovery.
	Files []string
	// EnvFile overrides the project .env file used for interpolation.
	EnvFile string
	// Environ is the process environment as KEY=VALUE pairs.
	Environ []string
	// Profiles are the profiles to enable; "*" enables all. When empty,
	// COMPOSE_PROFILES from the environment is used.
	Profiles []string
}

// Load discovers, parses and merges a Compose project. It never writes
// files, runs processes or opens network connections.
func Load(opts Options) (*Project, error) {
	files, dir, err := discover(opts)
	if err != nil {
		return nil, err
	}
	p := &Project{
		Dir:      dir,
		Services: map[string]*Service{},
		Volumes:  map[string]Location{},
		Networks: map[string]Location{},
		env:      map[string]string{},
	}
	if err := p.loadEnv(opts); err != nil {
		return nil, err
	}
	p.ActiveProfiles = opts.Profiles
	if len(p.ActiveProfiles) == 0 {
		if v := p.env["COMPOSE_PROFILES"]; v != "" {
			for _, s := range strings.Split(v, ",") {
				if s = strings.TrimSpace(s); s != "" {
					p.ActiveProfiles = append(p.ActiveProfiles, s)
				}
			}
		}
	}
	for _, f := range files {
		disp := p.Display(f)
		data, err := readRegularFile(f, MaxComposeFileSize)
		if err != nil {
			return nil, &LoadError{Loc: Location{File: disp}, Msg: "cannot read file: " + describeErr(err)}
		}
		root, err := parseYAML(disp, data)
		if err != nil {
			return nil, err
		}
		p.Files = append(p.Files, disp)
		p.scan(disp, root, "")
		if err := p.mergeFile(disp, root); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func describeErr(err error) string {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "no such file"
	case errors.Is(err, os.ErrPermission):
		return "permission denied"
	default:
		var pe *os.PathError
		if errors.As(err, &pe) {
			return pe.Err.Error()
		}
		return err.Error()
	}
}

func discover(opts Options) (files []string, dir string, err error) {
	path := opts.Path
	if path == "" {
		path = "."
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", &LoadError{Loc: Location{File: path}, Msg: err.Error()}
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return nil, "", &LoadError{Loc: Location{File: filepath.ToSlash(path)}, Msg: "cannot open path: " + describeErr(err)}
	}
	base := abs
	if !fi.IsDir() {
		if len(opts.Files) > 0 {
			return nil, "", &LoadError{Loc: Location{File: filepath.ToSlash(path)}, Msg: "path must be a directory when -f is used"}
		}
		return []string{abs}, filepath.Dir(abs), nil
	}
	if len(opts.Files) > 0 {
		for _, f := range opts.Files {
			if !filepath.IsAbs(f) {
				f = filepath.Join(base, f)
			}
			files = append(files, filepath.Clean(f))
		}
		return files, filepath.Dir(files[0]), nil
	}
	for _, name := range DefaultFiles {
		f := filepath.Join(base, name)
		if st, err := os.Stat(f); err == nil && st.Mode().IsRegular() {
			files = append(files, f)
			ext := filepath.Ext(name)
			stem := strings.TrimSuffix(name, ext)
			for _, oext := range []string{ext, otherExt(ext)} {
				o := filepath.Join(base, stem+".override"+oext)
				if st, err := os.Stat(o); err == nil && st.Mode().IsRegular() {
					files = append(files, o)
					break
				}
			}
			return files, base, nil
		}
	}
	return nil, "", &LoadError{
		Loc: Location{File: filepath.ToSlash(path)},
		Msg: "no Compose file found (looked for " + strings.Join(DefaultFiles, ", ") + ")",
	}
}

func otherExt(ext string) string {
	if ext == ".yaml" {
		return ".yml"
	}
	return ".yaml"
}

func (p *Project) loadEnv(opts Options) error {
	dotenv := filepath.Join(p.Dir, ".env")
	explicit := opts.EnvFile != ""
	if explicit {
		dotenv = opts.EnvFile
		if !filepath.IsAbs(dotenv) {
			dotenv, _ = filepath.Abs(dotenv)
		}
	}
	data, err := readRegularFile(dotenv, MaxAuxFileSize)
	switch {
	case err == nil:
		p.DotenvFile = p.Display(dotenv)
		p.loadDotenv(data, opts.Environ)
	case explicit || !errors.Is(err, os.ErrNotExist):
		return &LoadError{Loc: Location{File: p.Display(dotenv)}, Msg: "cannot read env file: " + describeErr(err)}
	}
	// The process environment takes precedence over .env, like Docker Compose.
	for _, kv := range opts.Environ {
		if k, v, ok := strings.Cut(kv, "="); ok && k != "" {
			p.env[k] = v
		}
	}
	if data, err := readRegularFile(filepath.Join(p.Dir, ".env.example"), MaxAuxFileSize); err == nil {
		p.envExample = map[string]bool{}
		for k := range ParseDotenv(data) {
			p.envExample[k] = true
		}
	}
	return nil
}

// Resolve returns the absolute path of a path relative to the project directory.
func (p *Project) Resolve(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(p.Dir, filepath.FromSlash(path))
}

// Display returns path relative to the project directory with forward slashes.
func (p *Project) Display(path string) string {
	if rel, err := filepath.Rel(p.Dir, path); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

// EnvFileNames returns the variable names declared in an env_file (never
// the values). Missing or unreadable files yield nil.
func (p *Project) EnvFileNames(path string) []string {
	data, err := readRegularFile(p.Resolve(path), MaxAuxFileSize)
	if err != nil {
		return nil
	}
	var names []string
	for k := range ParseDotenv(data) {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func (p *Project) lookupEnv(name string) (string, bool) {
	v, ok := p.env[name]
	return v, ok
}

// scan walks every value scalar (not following aliases, so each source line
// is visited once) to collect variable references, interpolation errors and
// duplicate keys.
func (p *Project) scan(file string, n *yaml.Node, path string) {
	switch n.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for i, c := range n.Content {
			sub := path
			if n.Kind == yaml.SequenceNode {
				sub = fmt.Sprintf("%s[%d]", path, i)
			}
			p.scan(file, c, sub)
		}
	case yaml.MappingNode:
		seen := map[string]*yaml.Node{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			sub := joinPath(path, k.Value)
			if k.Kind == yaml.ScalarNode && k.Value != "<<" {
				if first, dup := seen[k.Value]; dup {
					p.DupKeys = append(p.DupKeys, DuplicateKey{Key: k.Value, Path: sub, Loc: loc(file, k), First: loc(file, first)})
				} else {
					seen[k.Value] = k
				}
			}
			p.scan(file, v, sub)
		}
	case yaml.ScalarNode:
		if !strings.Contains(n.Value, "$") || n.Tag == "!!binary" {
			return
		}
		_, refs, err := Interpolate(n.Value, p.lookupEnv)
		l := loc(file, n)
		for _, r := range refs {
			p.VarUses = append(p.VarUses, VarUse{Ref: r, Loc: l, Path: path})
		}
		if err != nil {
			var ie *InterpError
			msg := err.Error()
			if errors.As(err, &ie) {
				msg = ie.Msg
			}
			p.InterpErrors = append(p.InterpErrors, InterpProblem{Loc: l, Path: path, Msg: msg})
		}
	}
}

func joinPath(base, key string) string {
	if base == "" {
		return key
	}
	return base + "." + key
}

func (p *Project) value(file string, n *yaml.Node) Value {
	v := Value{Raw: n.Value, Loc: loc(file, n), Interpolated: HasInterpolation(n.Value)}
	out, refs, err := Interpolate(n.Value, p.lookupEnv)
	v.Value = out
	if err != nil {
		v.Unresolved = true
		v.Value = ""
	}
	for _, r := range refs {
		if r.Problem {
			v.Unresolved = true
		}
	}
	return v
}

func (p *Project) mergeFile(file string, root *yaml.Node) error {
	for _, kv := range pairs(root) {
		val := resolve(kv.Val)
		switch kv.Key.Value {
		case "version":
			p.VersionKeys = append(p.VersionKeys, loc(file, kv.Key))
		case "volumes", "networks":
			target := p.Volumes
			if kv.Key.Value == "networks" {
				target = p.Networks
			}
			if hasTag(kv.Val, "!reset") || hasTag(kv.Val, "!override") {
				for name := range target {
					delete(target, name)
				}
			}
			for _, e := range pairs(val) {
				if _, ok := target[e.Key.Value]; !ok {
					target[e.Key.Value] = loc(file, e.Key)
				}
			}
		case "services":
			if isNull(val) {
				continue
			}
			if val == nil || val.Kind != yaml.MappingNode {
				return &LoadError{Loc: loc(file, kv.Key), Msg: "\"services\" must be a mapping"}
			}
			if hasTag(kv.Val, "!reset") || hasTag(kv.Val, "!override") {
				p.Services = map[string]*Service{}
			}
			for _, s := range pairs(val) {
				if hasTag(s.Val, "!reset") {
					delete(p.Services, s.Key.Value)
					continue
				}
				if hasTag(s.Val, "!override") {
					delete(p.Services, s.Key.Value)
				}
				p.mergeService(file, s.Key, resolve(s.Val))
			}
		}
	}
	return nil
}

func hasTag(n *yaml.Node, tag string) bool {
	return n != nil && (n.Tag == tag || (resolve(n) != nil && resolve(n).Tag == tag))
}

func (p *Project) mergeService(file string, key, n *yaml.Node) {
	name := key.Value
	s, ok := p.Services[name]
	if !ok {
		s = &Service{Name: name, Loc: loc(file, key)}
		p.Services[name] = s
	}
	for _, kv := range pairs(n) {
		k, raw := kv.Key.Value, kv.Val
		v := resolve(raw)
		if v == nil {
			continue
		}
		reset := raw.Tag == "!reset" || (v != nil && v.Tag == "!reset")
		override := raw.Tag == "!override" || (v != nil && v.Tag == "!override")
		if reset {
			resetField(s, k)
			continue
		}
		if override {
			resetField(s, k)
		}
		switch k {
		case "image":
			if v.Kind == yaml.ScalarNode {
				val := p.value(file, v)
				s.Image = &val
			}
		case "build":
			s.Build = p.build(file, kv.Key, v, s.Build)
		case "extends":
			s.Extends = true
		case "env_file":
			p.mergeEnvFiles(file, s, v)
		case "ports":
			for _, it := range items(v) {
				p.addPort(file, s, it)
			}
		case "volumes":
			for _, it := range items(v) {
				p.addVolume(file, s, it)
			}
		case "networks":
			p.mergeNetworks(file, s, v)
		case "depends_on":
			p.mergeDepends(file, s, v)
		case "healthcheck":
			s.Healthcheck = healthcheck(file, kv.Key, v, s.Healthcheck)
		case "profiles":
			for _, it := range items(v) {
				if it.Kind == yaml.ScalarNode {
					if name := p.value(file, it).Value; !slices.Contains(s.Profiles, name) {
						s.Profiles = append(s.Profiles, name)
					}
				}
			}
		case "network_mode":
			if v.Kind == yaml.ScalarNode {
				val := p.value(file, v)
				s.NetworkMode = &val
			}
		}
	}
}

func resetField(s *Service, k string) {
	switch k {
	case "image":
		s.Image = nil
	case "build":
		s.Build = nil
	case "env_file":
		s.EnvFiles = nil
	case "ports":
		s.Ports = nil
	case "volumes":
		s.Volumes = nil
	case "networks":
		s.Networks = nil
	case "depends_on":
		s.DependsOn = nil
	case "healthcheck":
		s.Healthcheck = nil
	case "profiles":
		s.Profiles = nil
	case "network_mode":
		s.NetworkMode = nil
	}
}

// build merges a build section onto prev (from an earlier file) field by
// field, as Docker Compose merges mappings.
func (p *Project) build(file string, key, n *yaml.Node, prev *Build) *Build {
	b := &Build{Loc: loc(file, key), Context: Value{Raw: ".", Value: ".", Loc: loc(file, key)}}
	if prev != nil {
		cp := *prev
		b = &cp
	}
	switch {
	case n == nil || isNull(n):
		return nil
	case n.Kind == yaml.ScalarNode:
		b.Context = p.value(file, n)
	case n.Kind == yaml.MappingNode:
		if _, c := lookup(n, "context"); c != nil && c.Kind == yaml.ScalarNode {
			b.Context = p.value(file, c)
		}
		if _, d := lookup(n, "dockerfile"); d != nil && d.Kind == yaml.ScalarNode {
			v := p.value(file, d)
			b.Dockerfile = &v
		}
		if _, d := lookup(n, "dockerfile_inline"); d != nil {
			b.Inline = true
		}
	default:
		return nil
	}
	return b
}

func (p *Project) mergeEnvFiles(file string, s *Service, n *yaml.Node) {
	add := func(ref EnvFileRef) {
		for i, e := range s.EnvFiles {
			if e.Path.Value == ref.Path.Value {
				s.EnvFiles[i] = ref
				return
			}
		}
		s.EnvFiles = append(s.EnvFiles, ref)
	}
	var entries []*yaml.Node
	if n != nil && n.Kind == yaml.ScalarNode && !isNull(n) {
		entries = []*yaml.Node{n}
	} else {
		entries = items(n)
	}
	for _, e := range entries {
		switch e.Kind {
		case yaml.ScalarNode:
			add(EnvFileRef{Path: p.value(file, e), Required: true})
		case yaml.MappingNode:
			_, pathNode := lookup(e, "path")
			if pathNode == nil || pathNode.Kind != yaml.ScalarNode {
				continue
			}
			ref := EnvFileRef{Path: p.value(file, pathNode), Required: true}
			if _, r := lookup(e, "required"); r != nil && r.Kind == yaml.ScalarNode {
				if b, err := strconv.ParseBool(p.value(file, r).Value); err == nil {
					ref.Required = b
				}
			}
			add(ref)
		}
	}
}

func (p *Project) addPort(file string, s *Service, n *yaml.Node) {
	port := Port{Loc: loc(file, n)}
	switch n.Kind {
	case yaml.ScalarNode:
		v := p.value(file, n)
		port.Short = &v
	case yaml.MappingNode:
		for field, dst := range map[string]**Value{"target": &port.Target, "published": &port.Published, "host_ip": &port.HostIP, "protocol": &port.Protocol} {
			if _, f := lookup(n, field); f != nil && f.Kind == yaml.ScalarNode && !isNull(f) {
				v := p.value(file, f)
				*dst = &v
			}
		}
	default:
		return
	}
	for _, existing := range s.Ports {
		if existing.key() == port.key() {
			return
		}
	}
	s.Ports = append(s.Ports, port)
}

func (p *Project) addVolume(file string, s *Service, n *yaml.Node) {
	vol := Volume{Loc: loc(file, n)}
	switch n.Kind {
	case yaml.ScalarNode:
		v := p.value(file, n)
		src, target := SplitShortVolume(v.Value)
		vol.Short, vol.Target = true, target
		vol.Source = Value{Raw: v.Raw, Value: src, Loc: v.Loc, Interpolated: v.Interpolated, Unresolved: v.Unresolved}
		vol.Type = shortVolumeType(src)
	case yaml.MappingNode:
		if _, t := lookup(n, "type"); t != nil && t.Kind == yaml.ScalarNode {
			vol.Type = p.value(file, t).Value
		}
		if _, src := lookup(n, "source"); src != nil && src.Kind == yaml.ScalarNode {
			vol.Source = p.value(file, src)
		}
		if _, tg := lookup(n, "target"); tg != nil && tg.Kind == yaml.ScalarNode {
			vol.Target = p.value(file, tg).Value
		}
	default:
		return
	}
	for i, existing := range s.Volumes {
		if vol.Target != "" && existing.Target == vol.Target {
			s.Volumes[i] = vol
			return
		}
	}
	s.Volumes = append(s.Volumes, vol)
}

// SplitShortVolume splits "SOURCE:TARGET[:MODE]" short volume syntax,
// keeping Windows drive letters ("C:\data:/data") in the source.
func SplitShortVolume(spec string) (source, target string) {
	drive := ""
	if len(spec) >= 3 && spec[1] == ':' && (spec[2] == '\\' || spec[2] == '/') && isLetter(spec[0]) {
		drive, spec = spec[:2], spec[2:]
	}
	parts := strings.Split(spec, ":")
	if len(parts) == 1 {
		if drive != "" {
			return "", drive + parts[0]
		}
		return "", parts[0]
	}
	return drive + parts[0], parts[1]
}

func shortVolumeType(src string) string {
	switch {
	case src == "":
		return "volume" // anonymous volume
	case strings.ContainsAny(src, "/\\") || strings.HasPrefix(src, ".") || strings.HasPrefix(src, "~") ||
		(len(src) >= 2 && src[1] == ':' && isLetter(src[0])):
		return "bind"
	default:
		return "volume"
	}
}

func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func (p *Project) mergeNetworks(file string, s *Service, n *yaml.Node) {
	add := func(name, shown string, l Location) {
		for _, e := range s.Networks {
			if e.Name == name {
				return
			}
		}
		s.Networks = append(s.Networks, NetworkRef{Name: name, Shown: shown, Loc: l})
	}
	if n == nil {
		return
	}
	switch n.Kind {
	case yaml.SequenceNode:
		for _, it := range items(n) {
			if it.Kind == yaml.ScalarNode {
				v := p.value(file, it)
				add(v.Value, shownValue(v), loc(file, it))
			}
		}
	case yaml.MappingNode:
		for _, kv := range pairs(n) {
			add(kv.Key.Value, kv.Key.Value, loc(file, kv.Key))
		}
	}
}

func (p *Project) mergeDepends(file string, s *Service, n *yaml.Node) {
	set := func(d Dependency) {
		for i, e := range s.DependsOn {
			if e.Service == d.Service {
				s.DependsOn[i] = d
				return
			}
		}
		s.DependsOn = append(s.DependsOn, d)
	}
	if n == nil {
		return
	}
	switch n.Kind {
	case yaml.SequenceNode:
		for _, it := range items(n) {
			if it.Kind == yaml.ScalarNode {
				v := p.value(file, it)
				set(Dependency{Service: v.Value, Shown: shownValue(v), Required: true, Short: true, Loc: loc(file, it)})
			}
		}
	case yaml.MappingNode:
		for _, kv := range pairs(n) {
			d := Dependency{Service: kv.Key.Value, Shown: kv.Key.Value, Required: true, Loc: loc(file, kv.Key)}
			for _, e := range s.DependsOn {
				if e.Service == d.Service && !e.Short && !hasTag(kv.Val, "!override") {
					d.Condition, d.Required = e.Condition, e.Required
				}
			}
			body := resolve(kv.Val)
			if _, c := lookup(body, "condition"); c != nil && c.Kind == yaml.ScalarNode {
				d.Condition = p.value(file, c).Value
			}
			if _, r := lookup(body, "required"); r != nil && r.Kind == yaml.ScalarNode {
				if b, err := strconv.ParseBool(p.value(file, r).Value); err == nil {
					d.Required = b
				}
			}
			set(d)
		}
	}
}

// healthcheck merges a healthcheck section onto prev field by field.
func healthcheck(file string, key, n *yaml.Node, prev *Healthcheck) *Healthcheck {
	h := &Healthcheck{Loc: loc(file, key)}
	if prev != nil {
		h.disableKey, h.testNone = prev.disableKey, prev.testNone
	}
	if _, d := lookup(n, "disable"); d != nil && d.Kind == yaml.ScalarNode {
		b, err := strconv.ParseBool(d.Value)
		h.disableKey = err == nil && b
	}
	if _, t := lookup(n, "test"); t != nil {
		h.testNone = false
		switch {
		case t.Kind == yaml.ScalarNode && strings.EqualFold(strings.TrimSpace(t.Value), "NONE"):
			h.testNone = true
		case t.Kind == yaml.SequenceNode:
			if it := items(t); len(it) > 0 && it[0].Kind == yaml.ScalarNode && strings.EqualFold(it[0].Value, "NONE") {
				h.testNone = true
			}
		}
	}
	h.Disabled = h.disableKey || h.testNone
	return h
}

// loadDotenv stores .env values, interpolating unquoted and double-quoted
// values from the process environment and earlier .env entries, as Docker
// Compose does. Single-quoted values are literal.
func (p *Project) loadDotenv(data []byte, environ []string) {
	osEnv := map[string]string{}
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok && k != "" {
			osEnv[k] = v
		}
	}
	for _, e := range parseDotenvEntries(data) {
		v := e.Value
		if !e.Literal {
			lookup := func(name string) (string, bool) {
				if val, ok := osEnv[name]; ok {
					return val, true
				}
				val, ok := p.env[name]
				return val, ok
			}
			expanded, refs, err := Interpolate(v, lookup)
			if err != nil || len(expanded) > MaxAuxFileSize || failsRequired(refs) {
				// Leave the variable unset so its uses are reported instead
				// of silently trusting an empty or runaway value.
				continue
			}
			v = expanded
		}
		p.env[e.Key] = v
	}
}

// shownValue is the text rules may print for v: never an interpolated value.
func shownValue(v Value) string {
	if v.Interpolated {
		return v.Raw
	}
	return v.Value
}

func failsRequired(refs []VarRef) bool {
	for _, r := range refs {
		if r.Problem && r.Required() {
			return true
		}
	}
	return false
}
