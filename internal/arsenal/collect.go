package arsenal

import (
	"fmt"
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
	// openName stands where a name would, in the line of an open list.
	openName = "(open)"
)

// Target is another repository a launch may go into: its primary checkout.
type Target struct {
	FS   fs.FS
	Path string
}

// Collect reads the inventory of the project whose primary checkout is
// fsys (at the real path primary) and of the user folder home. With a target
// it is what a build launched into that repository may name
// (catalog.ScanTarget), and the first line says so. Only an invalid
// jitso/jitso.yaml, or an unreadable instructions folder, is an error; an
// invalid user file is one line of the inventory.
//
// For an open allowlist the inventory says so, one line of the kind with the
// name (open), and lists what exists in its place: nothing for models, efforts
// and tools, which are free text, and the skill folders and servers a build
// may name for skills and mcp. An open list leaves no candidate.
func Collect(fsys fs.FS, primary, home string, target *Target) (Inventory, error) {
	// items is the repository whose skills and servers a build names.
	items := fsys
	var res *catalog.Result
	var err error
	if target == nil {
		res, err = catalog.Scan(fsys, primary)
	} else {
		items = target.FS
		res, err = catalog.ScanTarget(fsys, primary, "", target.FS)
	}
	if err != nil {
		return Inventory{}, err
	}
	cfg := res.Config
	// The scan above reads no user layer; the inventory reads it on its own,
	// so that an invalid file is a line and not an error.
	cfg.User = catalog.LoadUser(home)
	user := cfg.User
	userKnown := user.Err == nil && home != ""
	var inv Inventory
	if target != nil {
		inv.Header = fmt.Sprintf("%-8s %s (skills, mcp, tools and routes; models, efforts, fields and subagents of %s)", "target", target.Path, primary)
	}
	add := func(kind, name, detail string) { inv.Allowed = append(inv.Allowed, Item{kind, name, detail}) }
	candidate := func(kind, name, detail string) { inv.Candidates = append(inv.Candidates, Item{kind, name, detail}) }
	open := func(kind, what, key, file string) { add(kind, openName, what+": no "+key+" list in "+file) }
	values := func(kind, key, what string, list []string) {
		if catalog.Open(list) {
			open(kind, what, key, catalog.ConfigPath)
		}
		for _, v := range list {
			add(kind, v, "")
		}
	}

	values("model", "models", "any model id", cfg.Models)
	values("effort", "efforts", "any effort name", cfg.Efforts)
	values("tool", "tools", "any tool name", cfg.Tools)

	skills := cfg.Skills
	if catalog.Open(cfg.Skills) {
		open("skill", "any skill folder of the project", "skills", catalog.ConfigPath)
		skills = without(cfg.ProjectSkillDirs, func(s string) bool { return s == catalog.JitsoSkill })
	}
	for _, s := range skills {
		add("skill", s, projectSkillSummary(items, s))
	}
	if !slices.Contains(cfg.Skills, catalog.JitsoSkill) {
		add("skill", catalog.JitsoSkill, "built in; travels with conductor builds and builds that name it")
	}
	if user.Err != nil {
		add("user", strings.TrimPrefix(fail.Text(user.Err), fail.EnvPrefix), "")
	} else {
		userSkills := user.Skills
		if userKnown && catalog.Open(user.Skills) {
			open("skill", "user:<name> of any skill under "+catalog.UserSkillsDisplay, "skills", catalog.UserConfigDisplay)
			userSkills = without(userSkillDirs(home), cfg.OwnsSkill)
		}
		for _, s := range userSkills {
			add("skill", "user:"+s, userSkillSummary(user, s))
		}
	}

	project := cfg.ProjectServers
	servers := cfg.MCP
	if catalog.Open(cfg.MCP) {
		open("mcp", "any server of .mcp.json", "mcp", catalog.ConfigPath)
		servers = serverNames(project)
	}
	for _, name := range servers {
		detail := "(not defined in .mcp.json)"
		if def, ok := project[name]; ok {
			detail = Transport(def)
		}
		add("mcp", name, detail)
	}
	if user.Err == nil {
		userMCP := user.MCP
		if userKnown && catalog.Open(user.MCP) {
			open("mcp", "user:<name> of any server of "+catalog.UserClaudeDisplay, "mcp", catalog.UserConfigDisplay)
			userMCP = without(user.ServerNames(), cfg.OwnsMCP)
		}
		for _, name := range userMCP {
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

	if !catalog.Open(cfg.Skills) {
		for _, s := range dirNames(items, ".agents/skills") {
			if !slices.Contains(cfg.Skills, s) && s != catalog.JitsoSkill {
				candidate("skill", s, editProject)
			}
		}
	}
	if user.Err == nil && !catalog.Open(user.Skills) {
		for _, s := range userSkillDirs(home) {
			if !slices.Contains(user.Skills, s) {
				candidate("skill", "user:"+s, editUser)
			}
		}
	}
	if !catalog.Open(cfg.MCP) {
		for _, name := range serverNames(project) {
			if !slices.Contains(cfg.MCP, name) {
				candidate("mcp", name, Transport(project[name])+", "+editProject)
			}
		}
	}
	if user.Err == nil && !catalog.Open(user.MCP) {
		for _, name := range user.ServerNames() {
			if !slices.Contains(user.MCP, name) {
				candidate("mcp", "user:"+name, userServerDetail(user, name)+", "+editUser)
			}
		}
	}
	candidate("cloud", "connectors", "listed by your own session (/mcp); a launched session cannot get them yet")
	return inv, nil
}

// without is names minus the ones drop reports.
func without(names []string, drop func(string) bool) []string {
	var out []string
	for _, n := range names {
		if !drop(n) {
			out = append(out, n)
		}
	}
	return out
}

// serverNames are the names of a server table, sorted.
func serverNames(servers map[string]map[string]any) []string {
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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
	if flags := catalog.LiteralArgFlags(name, def, user.LiteralEnv); len(flags) > 0 {
		detail += ", literal args after: " + strings.Join(flags, ", ") + " (refused at launch)"
	}
	return detail
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
