package catalog

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/jsconv"
)

// The user files, as messages name them.
const (
	UserConfigDisplay = "~/.jitso/jitso.yaml"
	UserSkillsDisplay = "~/.claude/skills"
	UserClaudeDisplay = "~/.claude.json"
)

// JitsoSkill is JITSO's own usage skill. skills_first may name it without an
// allowlist entry: a launch carries it when the project holds no copy.
const JitsoSkill = "jitso"

// ScopeUser is the scope of a name that resolves in the user allowlist.
const ScopeUser = "user"

// User is the user allowlist, ~/.jitso/jitso.yaml, and what the user folder
// holds for the names it lists. A plain name never resolves here: only a
// user:<name> of skills_first or mcp does. The zero value is an empty layer.
type User struct {
	// Home is $HOME, "" for no user layer at all.
	Home string
	// Skills are the folder names under ~/.claude/skills a build may name as
	// user:<name>; MCP the servers of ~/.claude.json it may name that way.
	Skills []string
	MCP    []string
	// LiteralEnv are the <server>/<KEY> entries whose literal env or header
	// value the owner declared not secret.
	LiteralEnv []string
	// Err is why the file is invalid. It fails only a command that needs the
	// layer: one whose builds name a user-scope item.
	Err error

	servers *userServers
}

type userServers struct {
	once  sync.Once
	table map[string]any
}

// LoadUser reads the user allowlist under home. A missing file is an empty
// layer; an invalid one is a layer whose Err is set.
func LoadUser(home string) User {
	u := User{Home: home, servers: &userServers{}}
	if home == "" {
		return u
	}
	data, err := os.ReadFile(filepath.Join(home, ".jitso", "jitso.yaml"))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			u.Err = userInvalid(err.Error())
		}
		return u
	}
	raw, err := decodeYAML(data)
	if err != nil {
		u.Err = userInvalid(err.Error())
		return u
	}
	if raw == nil {
		return u
	}
	m, ok := raw.(*Map)
	if !ok {
		u.Err = userInvalid("the top level is not a mapping")
		return u
	}
	u.Skills = jsconv.Strings(m.Value("skills"))
	u.MCP = jsconv.Strings(m.Value("mcp"))
	u.LiteralEnv = jsconv.Strings(m.Value("literal_env"))
	return u
}

func userInvalid(problem string) error {
	return fail.Env(UserConfigDisplay + " is invalid: " + problem)
}

// hasSkill reports whether ~/.claude/skills/<name>/SKILL.md is a file.
func (u User) hasSkill(name string) bool {
	if u.Home == "" || !plainName(name) {
		return false
	}
	info, err := os.Stat(filepath.Join(u.Home, ".claude", "skills", name, "SKILL.md"))
	return err == nil && info.Mode().IsRegular()
}

// SkillDir is the folder of the user skill name.
func (u User) SkillDir(name string) string {
	return filepath.Join(u.Home, ".claude", "skills", name)
}

// Server is the definition of the user MCP server name: its entry under the
// top-level mcpServers of ~/.claude.json, read once. A file that is missing
// or is not JSON defines no server.
func (u User) Server(name string) (map[string]any, bool) {
	if u.Home == "" || u.servers == nil {
		return nil, false
	}
	u.servers.once.Do(func() {
		data, err := os.ReadFile(filepath.Join(u.Home, ".claude.json"))
		if err != nil {
			return
		}
		var top struct {
			MCPServers map[string]any `json:"mcpServers"`
		}
		if json.Unmarshal(data, &top) == nil {
			u.servers.table = top.MCPServers
		}
	})
	s, ok := u.servers.table[name].(map[string]any)
	return s, ok
}

// SplitScope splits a scoped name, scope:name, at its first colon. A name
// without a colon is plain: it resolves in the project's own allowlists.
func SplitScope(name string) (scope, rest string, scoped bool) {
	scope, rest, scoped = strings.Cut(name, ":")
	if !scoped {
		return "", name, false
	}
	return scope, rest, true
}

// IsScoped reports whether name carries a scope.
func IsScoped(name string) bool {
	_, _, scoped := SplitScope(name)
	return scoped
}

// UserName is the name of a user:<name> item, and false for any other.
func UserName(name string) (string, bool) {
	scope, rest, scoped := SplitScope(name)
	if !scoped || scope != ScopeUser {
		return "", false
	}
	return rest, true
}

// plainName is a name that is one folder name: not empty, no separator, not
// a dot folder.
func plainName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`) && !strings.HasPrefix(name, ".")
}

// namesUser reports whether a decoded build names a user:<name> item in
// skills_first or mcp: the builds that need the user layer.
func namesUser(p *Map) bool {
	isUser := func(name string) bool { _, ok := UserName(name); return ok }
	for _, key := range []string{"skills_first", "mcp"} {
		if slices.ContainsFunc(jsconv.Strings(p.Value(key)), isUser) {
			return true
		}
	}
	return false
}

// projectSkillDirs are the folder names under .claude/skills and
// .agents/skills of the project, a link to a folder included.
func projectSkillDirs(fsys fs.FS) []string {
	var names []string
	for _, dir := range []string{".claude/skills", ".agents/skills"} {
		entries, err := fs.ReadDir(fsys, dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !slices.Contains(names, e.Name()) {
				names = append(names, e.Name())
			}
		}
	}
	return names
}

// scopedSkillProblem is the problem of one scoped skills_first item, "" for
// none.
func scopedSkillProblem(item string, cfg Config) string {
	scope, name, _ := SplitScope(item)
	if scope != ScopeUser {
		return `skill "` + item + `": unknown scope (use user:)`
	}
	u := cfg.User
	switch {
	case u.Err != nil:
		return strings.TrimPrefix(u.Err.Error(), fail.EnvPrefix)
	case !slices.Contains(u.Skills, name):
		return `skill "` + item + `" is not in the user allowlist (` + UserConfigDisplay + `)`
	case !u.hasSkill(name):
		return `user skill "` + name + `" has no SKILL.md under ` + UserSkillsDisplay + "/" + name
	case slices.Contains(cfg.Skills, name) || slices.Contains(cfg.ProjectSkillDirs, name):
		return `user skill "` + name + `" has the name of a project skill`
	}
	return ""
}

// scopedMCPProblem is the problem of one scoped mcp item, "" for none.
func scopedMCPProblem(item string, cfg Config) string {
	scope, name, _ := SplitScope(item)
	switch scope {
	case ScopeUser:
	case "cloud":
		return `mcp "` + item + `": cloud connectors cannot be given to a launched session yet`
	default:
		return `mcp "` + item + `": unknown scope (use user:)`
	}
	u := cfg.User
	switch {
	case u.Err != nil:
		return strings.TrimPrefix(u.Err.Error(), fail.EnvPrefix)
	case !slices.Contains(u.MCP, name):
		return `mcp "` + item + `" is not in the user allowlist (` + UserConfigDisplay + `)`
	case slices.Contains(cfg.MCP, name):
		return `user mcp "` + name + `" has the name of a project mcp server`
	}
	if _, ok := u.Server(name); !ok {
		return `user mcp "` + name + `" is not defined in ` + UserClaudeDisplay
	}
	return ""
}
