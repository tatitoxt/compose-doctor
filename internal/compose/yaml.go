package compose

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"

	"gopkg.in/yaml.v3"
)

const (
	// MaxComposeFileSize bounds the size of a single Compose file.
	MaxComposeFileSize = 4 << 20
	// MaxDepth bounds YAML nesting depth.
	MaxDepth = 100
	// maxAliasDepth bounds alias/merge-key chains when resolving nodes.
	maxAliasDepth = 32
)

// Location is a position in a Compose file. File is relative to the project
// directory and uses forward slashes.
type Location struct {
	File   string `json:"file"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
}

func (l Location) String() string {
	switch {
	case l.Line > 0 && l.Column > 0:
		return fmt.Sprintf("%s:%d:%d", l.File, l.Line, l.Column)
	case l.Line > 0:
		return fmt.Sprintf("%s:%d", l.File, l.Line)
	default:
		return l.File
	}
}

// LoadError means the project could not be analysed at all.
type LoadError struct {
	Loc Location
	Msg string
}

func (e *LoadError) Error() string { return e.Loc.String() + ": " + e.Msg }

var yamlLineRe = regexp.MustCompile(`line (\d+)(?:, column (\d+))?: `)

func parseYAML(file string, data []byte) (root *yaml.Node, err error) {
	defer func() {
		if r := recover(); r != nil {
			root, err = nil, &LoadError{Loc: Location{File: file}, Msg: fmt.Sprintf("invalid YAML: parser panic: %v", r)}
		}
	}()
	if len(bytes.TrimSpace(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))) == 0 {
		return nil, &LoadError{Loc: Location{File: file}, Msg: "file is empty"}
	}
	var doc yaml.Node
	if err := yaml.NewDecoder(bytes.NewReader(data)).Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, &LoadError{Loc: Location{File: file}, Msg: "file is empty"}
		}
		return nil, yamlError(file, err)
	}
	if len(doc.Content) == 0 {
		return nil, &LoadError{Loc: Location{File: file}, Msg: "file is empty"}
	}
	root = doc.Content[0]
	if n := tooDeep(root, 0); n != nil {
		return nil, &LoadError{Loc: loc(file, n), Msg: fmt.Sprintf("YAML nesting deeper than %d levels", MaxDepth)}
	}
	if r := resolve(root); r == nil || r.Kind != yaml.MappingNode {
		return nil, &LoadError{Loc: loc(file, root), Msg: "top-level value must be a mapping"}
	}
	return root, nil
}

func yamlError(file string, err error) *LoadError {
	msg := err.Error()
	l := Location{File: file}
	if m := yamlLineRe.FindStringSubmatchIndex(msg); m != nil {
		l.Line, _ = strconv.Atoi(msg[m[2]:m[3]])
		if m[4] >= 0 {
			l.Column, _ = strconv.Atoi(msg[m[4]:m[5]])
		}
		msg = msg[m[1]:]
	} else if len(msg) > 6 && msg[:6] == "yaml: " {
		msg = msg[6:]
	}
	return &LoadError{Loc: l, Msg: "invalid YAML: " + msg}
}

func tooDeep(n *yaml.Node, depth int) *yaml.Node {
	if depth > MaxDepth {
		return n
	}
	for _, c := range n.Content {
		if d := tooDeep(c, depth+1); d != nil {
			return d
		}
	}
	return nil
}

func loc(file string, n *yaml.Node) Location {
	return Location{File: file, Line: n.Line, Column: n.Column}
}

// resolve follows alias nodes.
func resolve(n *yaml.Node) *yaml.Node {
	for i := 0; n != nil && n.Kind == yaml.AliasNode; i++ {
		if i > maxAliasDepth {
			return nil
		}
		n = n.Alias
	}
	if n != nil && n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		return resolve(n.Content[0])
	}
	return n
}

type pair struct{ Key, Val *yaml.Node }

// pairs returns the key/value pairs of a mapping, applying "<<" merge keys.
// Explicit keys win over merged ones; for a merge sequence, earlier entries
// win over later ones (YAML merge key semantics).
func pairs(n *yaml.Node) []pair { return mergedPairs(n, 0) }

func mergedPairs(n *yaml.Node, depth int) []pair {
	n = resolve(n)
	if n == nil || n.Kind != yaml.MappingNode || depth > maxAliasDepth {
		return nil
	}
	var explicit, merged []pair
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if k.Kind == yaml.ScalarNode && k.Value == "<<" && k.Tag != "!!str" {
			src := resolve(v)
			if src == nil {
				continue
			}
			if src.Kind == yaml.SequenceNode {
				for _, item := range src.Content {
					merged = appendMissing(merged, mergedPairs(item, depth+1))
				}
			} else {
				merged = appendMissing(merged, mergedPairs(src, depth+1))
			}
			continue
		}
		explicit = append(explicit, pair{k, v})
	}
	return appendMissing(explicit, merged)
}

func appendMissing(dst, src []pair) []pair {
	seen := make(map[string]bool, len(dst))
	for _, p := range dst {
		seen[p.Key.Value] = true
	}
	for _, p := range src {
		if !seen[p.Key.Value] {
			seen[p.Key.Value] = true
			dst = append(dst, p)
		}
	}
	return dst
}

func lookup(n *yaml.Node, key string) (k, v *yaml.Node) {
	for _, p := range pairs(n) {
		if p.Key.Value == key {
			return p.Key, resolve(p.Val)
		}
	}
	return nil, nil
}

func items(n *yaml.Node) []*yaml.Node {
	n = resolve(n)
	if n == nil || n.Kind != yaml.SequenceNode {
		return nil
	}
	out := make([]*yaml.Node, 0, len(n.Content))
	for _, c := range n.Content {
		if r := resolve(c); r != nil {
			out = append(out, r)
		}
	}
	return out
}

func isNull(n *yaml.Node) bool {
	return n == nil || (n.Kind == yaml.ScalarNode && n.Tag == "!!null")
}
