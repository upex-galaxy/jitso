package catalog

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/project"
)

// UserConfigFile is the user's settings file inside project.UserDir. Only this
// file can trust a parent folder: a project's own files are never read for
// trust, so a repository cannot widen its own catalog (docs/decisions.md).
const UserConfigFile = "config.yaml"

// userConfigShown is the file as the error text names it.
const userConfigShown = "~/.jitso/" + UserConfigFile

// UserConfig is ~/.jitso/config.yaml:
//
//	version: 1
//	trusted_parents:
//	  - ~/clients
//	  - /work/monorepo-root
type UserConfig struct {
	// TrustedParents are the real paths of the trusted folders that exist, in
	// the order of the file, without repeats. An entry that does not exist is
	// skipped silently.
	TrustedParents []string
}

// UserConfigPath is the file of the user's settings under home.
func UserConfigPath(home string) string {
	return filepath.Join(project.UserDir(home), UserConfigFile)
}

// LoadUserConfig reads ~/.jitso/config.yaml. A missing file, or an empty home,
// is a config with no trusted parent. An invalid file is an Env error (exit 3)
// `~/.jitso/config.yaml is invalid: <problems joined by "; ">`, because it
// decides which builds exist.
func LoadUserConfig(home string) (UserConfig, error) {
	if home == "" {
		return UserConfig{}, nil
	}
	data, err := os.ReadFile(UserConfigPath(home))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return UserConfig{}, nil
		}
		return UserConfig{}, err
	}
	cfg, problems := parseUserConfig(data, home)
	if len(problems) > 0 {
		return UserConfig{}, fail.Envf("%s is invalid: %s", userConfigShown, strings.Join(problems, "; "))
	}
	return cfg, nil
}

// parseUserConfig decodes and checks the settings. Problems come in the order
// unknown keys (document order), version, trusted_parents (entry order).
func parseUserConfig(data []byte, home string) (UserConfig, []string) {
	raw, err := decodeYAML(data)
	if err != nil {
		return UserConfig{}, []string{err.Error()}
	}
	m, ok := raw.(*Map)
	if !ok {
		return UserConfig{}, []string{"the top level is not a mapping"}
	}
	var problems []string
	for _, key := range m.Keys() {
		if key != "version" && key != "trusted_parents" {
			problems = append(problems, `unknown key "`+key+`"`)
		}
	}
	if v, present := m.Get("version"); present && !isOne(v) {
		problems = append(problems, "version must be 1")
	}
	var cfg UserConfig
	if v, present := m.Get("trusted_parents"); present {
		entries, ok := stringList(v)
		if !ok {
			problems = append(problems, "trusted_parents must be a list of folders")
		}
		for _, entry := range entries {
			dir, problem := resolveTrusted(entry, home)
			if problem != "" {
				problems = append(problems, problem)
				continue
			}
			if dir != "" && !slices.Contains(cfg.TrustedParents, dir) {
				cfg.TrustedParents = append(cfg.TrustedParents, dir)
			}
		}
	}
	return cfg, problems
}

// isOne is true for the number 1 and for no other value (a string "1" is not
// a version).
func isOne(v any) bool {
	switch x := v.(type) {
	case int:
		return x == 1
	case int64:
		return x == 1
	case uint64:
		return x == 1
	case float64:
		return x == 1
	}
	return false
}

// stringList is the value as a list of strings, false when it is anything else
// (a null, a scalar, a list with a non-string item).
func stringList(v any) ([]string, bool) {
	list, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		s, ok := item.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

// resolveTrusted turns one entry of trusted_parents into the real path of the
// folder, or a problem line. The folder is "" (and there is no problem) when
// the entry is fine but does not exist. The checks run in this order: the
// literal entries /, ~ and ~/; the form (absolute or ~/...); then the cleaned
// and the resolved path against the filesystem root and HOME.
func resolveTrusted(entry, home string) (dir, problem string) {
	tooBroad := `trusted parent "` + entry + `" is too broad (the filesystem root or HOME)`
	if entry == "/" || entry == "~" || entry == "~/" {
		return "", tooBroad
	}
	var expanded string
	switch {
	case strings.HasPrefix(entry, "~/"):
		expanded = filepath.Join(home, filepath.FromSlash(entry[2:]))
	case filepath.IsAbs(entry):
		expanded = entry
	default:
		return "", `trusted parent "` + entry + `" must be an absolute path or start with ~/`
	}
	cleaned := filepath.Clean(expanded)
	real, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		real = ""
	}
	for _, candidate := range []string{cleaned, real} {
		if candidate != "" && (isRoot(candidate) || sameFolder(candidate, home)) {
			return "", tooBroad
		}
	}
	return real, ""
}

// isRoot is true for the root of a filesystem.
func isRoot(p string) bool {
	return filepath.Dir(p) == p
}

// sameFolder is true when p is HOME, as written or as resolved.
func sameFolder(p, home string) bool {
	return p == filepath.Clean(home) || p == realPath(home)
}

// ParentsOf are the trusted folders that contain the project whose primary
// checkout has the real path primary, nearest first. The primary must be
// strictly inside a trusted folder: the folder itself and its siblings, also
// those that only share a name prefix, never match.
func (c UserConfig) ParentsOf(primary string) []string {
	var out []string
	for _, p := range c.TrustedParents {
		if _, ok := within(p, primary); ok {
			out = append(out, p)
		}
	}
	// Every match is an ancestor of the primary, so the longest path is the
	// nearest one.
	slices.SortFunc(out, func(a, b string) int { return len(b) - len(a) })
	return out
}
