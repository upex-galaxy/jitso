package catalog

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
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
// holds. A plain name never resolves here: only a user:<name> of skills_first
// or mcp does. Its skills and mcp lists open like the project's: without the
// file, or without items in a key, every user skill or server that exists may
// be named. literal_env is not an allowlist of that kind: without it no
// literal value travels. The zero value has no home, so nothing exists in it.
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
	// SettingsExclude and SettingsInclude are the settings block of the
	// file: extra top-level keys of the owner's Claude Code settings that
	// never travel with a launch, and keys that travel although the launch
	// would cut them. Names are not checked against any list of keys.
	SettingsExclude []string
	SettingsInclude []string
	// Err is why the file is invalid. It fails only a command that needs the
	// layer: one whose builds name a user-scope item.
	Err error

	servers *userServers
}

type userServers struct {
	once  sync.Once
	table map[string]any
}

// LoadUser reads the user allowlist under home. A missing or empty file opens
// both lists; an invalid one is a layer whose Err is set.
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
	u.Skills = allowlist(m.Value("skills"))
	u.MCP = allowlist(m.Value("mcp"))
	u.LiteralEnv = jsconv.Strings(m.Value("literal_env"))
	if problem := u.readSettings(m); problem != "" {
		u.Err = userInvalid(problem)
	}
	return u
}

// readSettings reads the optional settings block: a mapping that holds only
// exclude and include, each a list of text names. An absent or null block, and
// an absent or null list, are empty. It returns the problem text, "" for none.
func (u *User) readSettings(m *Map) string {
	v, present := m.Get("settings")
	if !present || v == nil {
		return ""
	}
	block, ok := v.(*Map)
	if !ok {
		return "settings must be a mapping with exclude and include lists"
	}
	for _, key := range block.Keys() {
		if key != "exclude" && key != "include" {
			return "settings has an unknown key " + key + " (use exclude or include)"
		}
	}
	for _, key := range []string{"exclude", "include"} {
		names, ok := keyNames(block.Value(key))
		if !ok {
			return "settings." + key + " must be a list of key names"
		}
		if key == "exclude" {
			u.SettingsExclude = names
		} else {
			u.SettingsInclude = names
		}
	}
	return ""
}

// keyNames reads a list of text names; null and an absent key are no names.
func keyNames(v any) ([]string, bool) {
	if v == nil || v == jsconv.Undefined {
		return nil, true
	}
	list, ok := v.([]any)
	if !ok {
		return nil, false
	}
	names := make([]string, 0, len(list))
	for _, item := range list {
		s, isText := item.(string)
		if !isText {
			return nil, false
		}
		names = append(names, s)
	}
	return names, true
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

// ServerNames are the server names under mcpServers of ~/.claude.json,
// sorted.
func (u User) ServerNames() []string {
	u.Server("")
	if u.servers == nil {
		return nil
	}
	names := make([]string, 0, len(u.servers.table))
	for name := range u.servers.table {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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

// ScopeCloud is the scope of a claude.ai connector, cloud:<name>, an item of
// mcp only.
const ScopeCloud = "cloud"

// CloudName is the connector name of a cloud:<name> item, and false for any
// other. The name may be empty: the validator reports that.
func CloudName(name string) (string, bool) {
	scope, rest, scoped := SplitScope(name)
	if !scoped || scope != ScopeCloud {
		return "", false
	}
	return rest, true
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
// skills_first or mcp, or a cloud:<name> item in mcp (it is checked against the
// same allowlist): the builds that need the user layer.
func namesUser(p *Map) bool {
	isUser := func(name string) bool { _, ok := UserName(name); return ok }
	isCloud := func(name string) bool { _, ok := CloudName(name); return ok }
	return slices.ContainsFunc(jsconv.Strings(p.Value("skills_first")), isUser) ||
		slices.ContainsFunc(jsconv.Strings(p.Value("mcp")), func(n string) bool { return isUser(n) || isCloud(n) })
}

// projectSkillDirs are the names of the folders under .agents/skills and
// .claude/skills of the project, a link to a folder included, sorted. A name
// that starts with a dot is not a skill.
func projectSkillDirs(fsys fs.FS) []string {
	var names []string
	for _, dir := range []string{".agents/skills", ".claude/skills"} {
		entries, err := fs.ReadDir(fsys, dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !plainName(e.Name()) || slices.Contains(names, e.Name()) {
				continue
			}
			if info, err := fs.Stat(fsys, path.Join(dir, e.Name())); err == nil && info.IsDir() {
				names = append(names, e.Name())
			}
		}
	}
	slices.Sort(names)
	return names
}

// projectServers is the mcpServers table of the project's .mcp.json, empty
// when the file is missing or is not JSON.
func projectServers(fsys fs.FS) map[string]map[string]any {
	data, err := fs.ReadFile(fsys, ".mcp.json")
	if err != nil {
		return nil
	}
	var top struct {
		MCPServers map[string]any `json:"mcpServers"`
	}
	if json.Unmarshal(data, &top) != nil {
		return nil
	}
	out := map[string]map[string]any{}
	for name, v := range top.MCPServers {
		def, _ := v.(map[string]any)
		out[name] = def
	}
	return out
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
	case !Open(u.Skills) && !slices.Contains(u.Skills, name):
		return `skill "` + item + `" is not in the user allowlist (` + UserConfigDisplay + `)`
	case !u.hasSkill(name):
		return `user skill "` + name + `" has no SKILL.md under ` + UserSkillsDisplay + "/" + name
	case cfg.OwnsSkill(name):
		return `user skill "` + name + `" has the name of a project skill`
	}
	return ""
}

// cloudMCPProblem is the problem of one cloud:<name> mcp item, "" for none. A
// connector is not checked against the machine here (that needs claude and is
// done at launch); the name must not be empty and, when the user allowlist
// lists mcp, the item as written must be listed (DECIDED: with its scope, so a
// connector never shares an entry with a user server of the same name).
func cloudMCPProblem(item, name string, cfg Config) string {
	u := cfg.User
	switch {
	case strings.TrimSpace(name) == "":
		return `mcp "` + item + `": a claude.ai connector needs a name (cloud:<name>)`
	case u.Err != nil:
		return strings.TrimPrefix(u.Err.Error(), fail.EnvPrefix)
	case !Open(u.MCP) && !slices.Contains(u.MCP, item):
		return `mcp "` + item + `" is not in the user allowlist (` + UserConfigDisplay + `)`
	}
	return ""
}

// scopedMCPProblem is the problem of one scoped mcp item, "" for none.
func scopedMCPProblem(item string, cfg Config) string {
	scope, name, _ := SplitScope(item)
	switch scope {
	case ScopeUser:
	case ScopeCloud:
		return cloudMCPProblem(item, name, cfg)
	default:
		return `mcp "` + item + `": unknown scope (use user:)`
	}
	u := cfg.User
	switch {
	case u.Err != nil:
		return strings.TrimPrefix(u.Err.Error(), fail.EnvPrefix)
	case !Open(u.MCP) && !slices.Contains(u.MCP, name):
		return `mcp "` + item + `" is not in the user allowlist (` + UserConfigDisplay + `)`
	case cfg.OwnsMCP(name):
		return `user mcp "` + name + `" has the name of a project mcp server`
	}
	def, ok := u.Server(name)
	if !ok {
		return `user mcp "` + name + `" is not defined in ` + UserClaudeDisplay
	}
	_, problem := UserServerCopy(name, def, u.LiteralEnv)
	return problem
}
