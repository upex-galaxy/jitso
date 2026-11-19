package arsenal

import (
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/fail"
)

const (
	editProject = "not allowlisted (edit " + catalog.ConfigPath + ")"
	editUser    = "not allowlisted (edit " + catalog.UserConfigDisplay + ")"
)

// Collect reads the inventory of the project whose primary checkout is
// fsys (at the real path primary) and of the user folder home. Only a missing
// or invalid jitso/jitso.yaml, or an unreadable instructions folder, is an
// error; an invalid user file is one line of the inventory.
func Collect(fsys fs.FS, primary, home string) (Inventory, error) {
	res, err := catalog.Scan(fsys, primary)
	if err != nil {
		return Inventory{}, err
	}
	cfg := res.Config
	user := catalog.LoadUser(home)
	var inv Inventory
	add := func(kind, name, detail string) { inv.Allowed = append(inv.Allowed, Item{kind, name, detail}) }
	candidate := func(kind, name, detail string) { inv.Candidates = append(inv.Candidates, Item{kind, name, detail}) }

	for _, m := range cfg.Models {
		add("model", m, "")
	}
	for _, e := range cfg.Efforts {
		add("effort", e, "")
	}
	for _, t := range cfg.Tools {
		add("tool", t, "")
	}
	for _, s := range cfg.Skills {
		add("skill", s, projectSkillSummary(fsys, s))
	}
	if !slices.Contains(cfg.Skills, catalog.JitsoSkill) {
		add("skill", catalog.JitsoSkill, "built in; travels with conductor builds and builds that name it")
	}
	if user.Err != nil {
		add("user", strings.TrimPrefix(fail.Text(user.Err), fail.EnvPrefix), "")
	} else {
		for _, s := range user.Skills {
			add("skill", "user:"+s, userSkillSummary(user, s))
		}
	}
	project := projectServers(fsys)
	for _, name := range cfg.MCP {
		detail := "(not defined in .mcp.json)"
		if def, ok := project[name]; ok {
			detail = Transport(def)
		}
		add("mcp", name, detail)
	}
	if user.Err == nil {
		for _, name := range user.MCP {
			add("mcp", "user:"+name, userServerDetail(user, name))
		}
	}
	for _, b := range res.Tolerant().Builds {
		if b.Tier == catalog.TierSubagent && b.Status == "approved" {
			add("subagent", b.ID, Summary(b.Description))
		}
	}
	for _, id := range res.RouteIDs {
		add("route", id, "")
	}
	for _, f := range cfg.Fields {
		add("field", f.Name, f.Source)
	}

	for _, s := range dirNames(fsys, ".agents/skills") {
		if !slices.Contains(cfg.Skills, s) && s != catalog.JitsoSkill {
			candidate("skill", s, editProject)
		}
	}
	if user.Err == nil {
		for _, s := range userSkillDirs(home) {
			if !slices.Contains(user.Skills, s) {
				candidate("skill", "user:"+s, editUser)
			}
		}
	}
	names := make([]string, 0, len(project))
	for name := range project {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !slices.Contains(cfg.MCP, name) {
			candidate("mcp", name, Transport(project[name])+", "+editProject)
		}
	}
	if user.Err == nil {
		for _, name := range user.ServerNames() {
			if !slices.Contains(user.MCP, name) {
				candidate("mcp", "user:"+name, userServerDetail(user, name)+", "+editUser)
			}
		}
	}
	candidate("cloud", "connectors", "listed by your own session (/mcp); a launched session cannot get them yet")
	return inv, nil
}

func projectSkillSummary(fsys fs.FS, name string) string {
	for _, dir := range []string{".agents/skills", ".claude/skills"} {
		if data, err := fs.ReadFile(fsys, path.Join(dir, name, "SKILL.md")); err == nil {
			return Summary(Description(string(data)))
		}
	}
	return "(no SKILL.md)"
}

func userSkillSummary(user catalog.User, name string) string {
	data, err := os.ReadFile(filepath.Join(user.SkillDir(name), "SKILL.md"))
	if err != nil {
		return "(no SKILL.md under " + catalog.UserSkillsDisplay + "/" + name + ")"
	}
	return Summary(Description(string(data)))
}

// userServerDetail is the transport of a user server and the key names that
// would refuse it, never a value.
func userServerDetail(user catalog.User, name string) string {
	def, ok := user.Server(name)
	if !ok {
		return "(not defined in " + catalog.UserClaudeDisplay + ")"
	}
	detail := Transport(def)
	if _, ok := def["oauth"]; ok {
		detail += ", uses OAuth (refused at launch)"
	}
	env, headers := catalog.LiteralKeys(name, def, user.LiteralEnv)
	if len(env) > 0 {
		detail += ", literal env: " + strings.Join(env, ", ") + " (refused at launch)"
	}
	if len(headers) > 0 {
		detail += ", literal headers: " + strings.Join(headers, ", ") + " (refused at launch)"
	}
	return detail
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

func dirNames(fsys fs.FS, dir string) []string {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || e.Type()&fs.ModeSymlink != 0 {
			names = append(names, e.Name())
		}
	}
	return names
}

// userSkillDirs are the folders under ~/.claude/skills that hold a SKILL.md,
// sorted.
func userSkillDirs(home string) []string {
	if home == "" {
		return nil
	}
	dir := filepath.Join(home, ".claude", "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if info, err := os.Stat(filepath.Join(dir, e.Name(), "SKILL.md")); err == nil && info.Mode().IsRegular() {
			names = append(names, e.Name())
		}
	}
	return names
}
