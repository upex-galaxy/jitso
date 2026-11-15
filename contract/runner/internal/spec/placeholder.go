package spec

import (
	"fmt"
	"regexp"
	"strings"
)

// Vars maps placeholder names to values.
type Vars map[string]string

// Built-in placeholder names (SPEC 4).
const (
	VarProject = "PROJECT"
	VarHome    = "HOME"
	VarBin     = "BIN"
	VarIndex   = "index"
	// VarJitso is supplied by the implementation adapter: the command that starts it.
	VarJitso = "JITSO"
)

var reserved = map[string]bool{VarProject: true, VarHome: true, VarBin: true, VarIndex: true, VarJitso: true}

// WorktreePrefix starts the placeholder of each linked worktree (`{{WORKTREE_<name>}}`).
const WorktreePrefix = "WORKTREE_"

// isReserved reports a name no capture and no for_each var may take.
func isReserved(name string) bool {
	return reserved[name] || strings.HasPrefix(name, WorktreePrefix)
}

var namePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// UnknownPlaceholder is returned for a well-formed `{{NAME}}` that has no value.
type UnknownPlaceholder struct{ Name string }

func (e *UnknownPlaceholder) Error() string {
	return fmt.Sprintf("unknown placeholder {{%s}}", e.Name)
}

// With returns a copy of v with the extra values set.
func (v Vars) With(extra Vars) Vars {
	out := make(Vars, len(v)+len(extra))
	for k, val := range v {
		out[k] = val
	}
	for k, val := range extra {
		out[k] = val
	}
	return out
}

// Resolve replaces every `{{NAME}}` in s. `{{{{` is a literal `{{`. A `{{` that opens neither
// is an error, so a typo never passes silently. Values are inserted verbatim, once.
func Resolve(s string, vars Vars) (string, error) {
	if !strings.Contains(s, "{{") {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		switch {
		case strings.HasPrefix(s[i:], "{{{{"):
			b.WriteString("{{")
			i += 4
		case strings.HasPrefix(s[i:], "{{"):
			end := strings.Index(s[i+2:], "}}")
			if end < 0 {
				return "", fmt.Errorf("unterminated placeholder in %q (write {{{{ for a literal {{)", excerpt(s, i))
			}
			name := s[i+2 : i+2+end]
			if !namePattern.MatchString(name) {
				return "", fmt.Errorf("malformed placeholder {{%s}} (write {{{{ for a literal {{)", name)
			}
			val, ok := vars[name]
			if !ok {
				return "", &UnknownPlaceholder{Name: name}
			}
			b.WriteString(val)
			i += end + 4
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String(), nil
}

// ResolveTree resolves every string (object keys included) of a decoded JSON tree.
func ResolveTree(v any, vars Vars) (any, error) {
	switch node := v.(type) {
	case string:
		return Resolve(node, vars)
	case []any:
		out := make([]any, len(node))
		for i, item := range node {
			r, err := ResolveTree(item, vars)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(node))
		for k, item := range node {
			rk, err := Resolve(k, vars)
			if err != nil {
				return nil, err
			}
			if _, dup := out[rk]; dup {
				return nil, fmt.Errorf("two keys resolve to %q", rk)
			}
			r, err := ResolveTree(item, vars)
			if err != nil {
				return nil, err
			}
			out[rk] = r
		}
		return out, nil
	}
	return v, nil
}

func excerpt(s string, at int) string {
	end := min(len(s), at+40)
	return s[at:end]
}
