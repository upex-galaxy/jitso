package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/instructions"
	"github.com/upex-galaxy/jitso/internal/project"
	"github.com/upex-galaxy/jitso/internal/settingsjson"
	"github.com/upex-galaxy/jitso/internal/skill"
)

const tracked = "ls-files --error-unmatch -- .claude/settings.json"

func TestHooksStates(t *testing.T) {
	dir := fixture(t)
	onPath := lookup(map[string]string{"jitso": "/usr/local/bin/jitso"})
	check := func(git map[string]int, look func(string) (string, error)) Line {
		c, _ := checksFor(t, dir, git)
		c.p.LookPath = look
		return c.hooks()
	}

	// No file, no JITSO handler: not registered.
	want(t, check(nil, onPath), Warn, "hooks", "not registered: run jitso setup hook")
	put(t, dir, ".claude/settings.json", `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "./notify.sh"}]}]}}`)
	want(t, check(nil, onPath), Warn, "hooks", "not registered: run jitso setup hook")

	// Registered, resolves and tracked: ok.
	put(t, dir, ".claude/settings.json", string(settingsjson.Fragment(settingsjson.Command)))
	got := check(map[string]int{tracked: 0}, onPath)
	want(t, got, OK, "hooks", "jitso hook guard is registered on PreToolUse (Bash, orca MCP) and SessionEnd in .claude/settings.json")
	if strings.Contains(got.Detail, ";") {
		t.Errorf("an ok line has nothing to add: %q", got.Detail)
	}

	// Not tracked: still says it is registered, and warns.
	got = check(map[string]int{tracked: 1}, onPath)
	want(t, got, Warn, "hooks", "registered on PreToolUse (Bash, orca MCP) and SessionEnd in .claude/settings.json; .claude/settings.json is not tracked by git: commit it")
	// A git that cannot answer is not a reason to warn.
	want(t, check(map[string]int{tracked: 128}, onPath), OK, "hooks", "is registered on")

	// A failure wins over the warnings.
	got = check(map[string]int{tracked: 1}, lookup(nil))
	want(t, got, Fail, "hooks", "jitso is not on PATH")
	if strings.Contains(got.Detail, "not tracked") {
		t.Errorf("a fail line holds the failure alone: %q", got.Detail)
	}

	// Broken file.
	put(t, dir, ".claude/settings.json", "{")
	got = check(nil, onPath)
	want(t, got, Fail, "hooks", ".claude/settings.json is not valid JSON")
	if strings.Contains(got.Detail, "nothing written") {
		t.Errorf("doctor writes nothing and does not say so: %q", got.Detail)
	}
	put(t, dir, ".claude/settings.json", "")
	want(t, check(nil, onPath), Fail, "hooks", "not valid JSON")
}

func TestHooksPartialAndDisabled(t *testing.T) {
	dir := fixture(t)
	c, _ := checksFor(t, dir, map[string]int{tracked: 0})
	c.p.LookPath = lookup(map[string]string{"jitso": "/x/jitso"})

	put(t, dir, ".claude/settings.json", `{"hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "jitso hook guard"}]}]}}`)
	want(t, c.hooks(), Warn, "hooks", "registered on PreToolUse Bash only (missing PreToolUse orca MCP, SessionEnd): run jitso setup hook")

	// A file with the Bash and SessionEnd groups but no orca MCP group.
	partial := strings.Replace(string(settingsjson.Fragment(settingsjson.Command)), `,
      {
        "matcher": "mcp__.*orca.*__.*",
        "hooks": [
          {
            "type": "command",
            "command": "jitso hook guard"
          }
        ]
      }`, "", 1)
	put(t, dir, ".claude/settings.json", partial)
	want(t, c.hooks(), Warn, "hooks", "registered on PreToolUse Bash, SessionEnd only (missing PreToolUse orca MCP): run jitso setup hook")

	// A JITSO handler that shares a group with another handler is not a group of its own.
	put(t, dir, ".claude/settings.json", `{"hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "jitso hook guard"}, {"type": "command", "command": "./x.sh"}]}]}}`)
	want(t, c.hooks(), Warn, "hooks", "but on none of")

	full := string(settingsjson.Fragment(settingsjson.Command))
	put(t, dir, ".claude/settings.json", strings.Replace(full, "{\n", "{\n  \"disableAllHooks\": true,\n", 1))
	want(t, c.hooks(), Warn, "hooks", "disableAllHooks is true in .claude/settings.json, so Claude Code runs no hook")
}

func TestHookCommandResolution(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "jitso")
	plain := filepath.Join(dir, "plain")
	os.WriteFile(exe, []byte("x"), 0o755)
	os.WriteFile(plain, []byte("x"), 0o644)
	spaced := filepath.Join(dir, "with space", "jitso")
	os.MkdirAll(filepath.Dir(spaced), 0o755)
	os.WriteFile(spaced, []byte("x"), 0o755)

	c := &projectChecks{p: Ports{GOOS: "linux", LookPath: lookup(map[string]string{"jitso": "/bin/jitso"})}.withDefaults()}
	cases := []struct {
		command string
		want    string
	}{
		{"jitso hook guard", ""},
		{"nosuch hook guard", "nosuch is not on PATH"},
		{"'" + exe + "' hook guard", ""},
		{"'" + spaced + "' hook guard", ""},
		{exe + " hook guard", ""},
		{"'/nonexistent/jitso' hook guard", "/nonexistent/jitso does not exist"},
		{"'" + plain + "' hook guard", plain + " is not executable"},
		{"'" + dir + "' hook guard", dir + " is a folder, not a program"},
		{"", "the hook command is empty"},
	}
	for _, tc := range cases {
		if got := c.resolveHook(tc.command); got != tc.want {
			t.Errorf("resolveHook(%q) = %q, want %q", tc.command, got, tc.want)
		}
	}
	// Windows has no execute bit.
	c.p.GOOS = "windows"
	if got := c.resolveHook("'" + plain + "' hook guard"); got != "" {
		t.Errorf("windows: %q", got)
	}
}

func TestFirstWord(t *testing.T) {
	cases := map[string]string{
		"jitso hook guard":            "jitso",
		"'/a b/jitso' hook guard":     "/a b/jitso",
		`"/a b/jitso" hook guard`:     "/a b/jitso",
		`'/it'\''s/jitso' hook guard`: "/it's/jitso",
		"/abs/jitso hook guard":       "/abs/jitso",
		"jitso":                       "jitso",
		"":                            "",
	}
	for in, want := range cases {
		if got := firstWord(in); got != want {
			t.Errorf("firstWord(%q) = %q, want %q", in, got, want)
		}
	}
}

// installSkill writes the shipped skill the way `install skill` does, with the
// given file data (nil means the shipped data) and the link.
func installSkill(t *testing.T, root, version string, files []skill.File, link bool) {
	t.Helper()
	if files == nil {
		files = skill.Files()
	}
	for _, f := range files {
		put(t, root, skill.RealRel+"/"+f.Path, string(f.Data))
	}
	data, err := skill.NewManifest(version, files).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	put(t, root, skill.RealRel+"/"+skill.ManifestName, string(data))
	if link {
		linkPath := filepath.Join(root, filepath.FromSlash(skill.LinkRel))
		os.MkdirAll(filepath.Dir(linkPath), 0o755)
		if err := os.Symlink(skill.LinkTarget, linkPath); err != nil {
			t.Fatal(err)
		}
	}
}

func oldFiles() []skill.File {
	files := skill.Files()
	files[0] = skill.File{Path: files[0].Path, Data: []byte("an older skill\n")}
	return files
}

func TestSkillProject(t *testing.T) {
	p := Ports{Version: "v9.9.9"}.withDefaults()
	root := t.TempDir()

	got, _ := checkSkill(p, "project", root, nil)
	want(t, got, Warn, "skill-project", "not installed (run jitso install skill --scope project; sessions that jitso launches load project skills only)")

	installSkill(t, root, "v9.9.9", nil, true)
	got, found := checkSkill(p, "project", root, nil)
	want(t, got, OK, "skill-project", "jitso skill v9.9.9 in .agents/skills/jitso and .claude/skills/jitso")
	if !found.Installed || found.Content == "" {
		t.Errorf("found = %+v", found)
	}

	// The link is gone, or leads elsewhere.
	link := filepath.Join(root, ".claude/skills/jitso")
	os.Remove(link)
	want(t, mustCheck(p, "project", root), Warn, "skill-project", "the link .claude/skills/jitso is missing (run jitso install skill --scope project)")
	os.MkdirAll(filepath.Join(root, "elsewhere"), 0o755)
	os.Symlink("../../elsewhere", link)
	want(t, mustCheck(p, "project", root), Warn, "skill-project", ".claude/skills/jitso points to ../../elsewhere, not to .agents/skills/jitso")
	os.Remove(link)

	// A second managed copy (the fallback where links cannot be made) counts.
	installSkillCopy := func() {
		for _, f := range skill.Files() {
			put(t, root, skill.LinkRel+"/"+f.Path, string(f.Data))
		}
		data, _ := skill.NewManifest("v9.9.9", skill.Files()).Marshal()
		put(t, root, skill.LinkRel+"/"+skill.ManifestName, string(data))
	}
	installSkillCopy()
	want(t, mustCheck(p, "project", root), OK, "skill-project", "jitso skill v9.9.9")
	put(t, root, skill.LinkRel+"/"+skill.Files()[0].Path, "edited copy\n")
	want(t, mustCheck(p, "project", root), Warn, "skill-project", ".claude/skills/jitso is a copy that is modified")

	// Edited by hand: the file is named.
	installSkill(t, root, "v9.9.9", nil, false)
	put(t, root, skill.RealRel+"/"+skill.Files()[0].Path, "mine\n")
	want(t, mustCheck(p, "project", root), Warn, "skill-project", "edited by hand in .agents/skills/jitso ("+skill.Files()[0].Path+")")
}

func mustCheck(p Ports, scope, root string) Line {
	line, _ := checkSkill(p, scope, root, nil)
	return line
}

func TestSkillOutdatedUnmanagedAndOther(t *testing.T) {
	p := Ports{Version: "v2.0.0"}.withDefaults()

	root := t.TempDir()
	installSkill(t, root, "v1.0.0", oldFiles(), true)
	want(t, mustCheck(p, "project", root), Warn, "skill-project", "out of date in .agents/skills/jitso: v1.0.0 installed, v2.0.0 shipped; run jitso install skill --scope project")

	foreign := t.TempDir()
	put(t, foreign, skill.RealRel+"/SKILL.md", "not ours\n")
	want(t, mustCheck(p, "project", foreign), Warn, "skill-project", "is not a copy jitso installed (no .jitso-skill.json); run jitso install skill --scope project --force")

	file := t.TempDir()
	put(t, file, skill.RealRel, "a file\n")
	want(t, mustCheck(p, "project", file), Warn, "skill-project", "is a file, not a folder jitso installed")

	// User scope: absent is fine, and the project's copy is compared.
	home := t.TempDir()
	user, _ := checkSkill(p, "user", home, nil)
	want(t, user, OK, "skill-user", "not installed (optional)")
	installSkill(t, home, "v2.0.0", nil, true)
	_, projectFound := checkSkill(p, "project", root, nil)
	user, _ = checkSkill(p, "user", home, &projectFound)
	want(t, user, Warn, "skill-user", "installed at both scopes with different content (a session that jitso launches reads the project copy)")
	same := skillFound{Installed: true, Content: skill.NewManifest("x", skill.Files()).ContentSHA256}
	user, _ = checkSkill(p, "user", home, &same)
	want(t, user, OK, "skill-user", "jitso skill v2.0.0 in ~/.agents/skills/jitso and ~/.claude/skills/jitso")
	user, _ = checkSkill(p, "user", home, &skillFound{})
	want(t, user, OK, "skill-user", "~/.agents/skills/jitso")
	user, _ = checkSkill(p, "user", "", nil)
	want(t, user, Warn, "skill-user", "HOME is not set")

	// A user copy that is out of date warns on its own.
	old := t.TempDir()
	installSkill(t, old, "v1.0.0", oldFiles(), true)
	want(t, mustCheck(p, "user", old), Warn, "skill-user", "out of date in ~/.agents/skills/jitso")
}

func block(t *testing.T, number int) string {
	t.Helper()
	text := instructions.Render(instructions.AddedNew, "\n") + "\n"
	return strings.Replace(text, "block=1", "block="+string(rune('0'+number)), 2)
}

func TestInstructionsProject(t *testing.T) {
	p := Ports{}.withDefaults()
	run := func(dir string) Line { return checkInstructions(p, project.Root{Primary: dir, Checkout: dir}) }

	dir := t.TempDir()
	want(t, run(dir), Warn, "instructions", "no AGENTS.md, so no JITSO block (run jitso setup instructions)")

	put(t, dir, "AGENTS.md", "# Rules\n")
	want(t, run(dir), Warn, "instructions", "no JITSO block in AGENTS.md (run jitso setup instructions)")

	put(t, dir, "AGENTS.md", block(t, 1))
	want(t, run(dir), OK, "instructions", "block 1 in AGENTS.md")

	// An older block number is replaced by a newer jitso.
	put(t, dir, "AGENTS.md", block(t, 0))
	want(t, run(dir), Warn, "instructions", "block 0 in AGENTS.md is older than block 1 (run jitso setup instructions)")

	// Broken markers fail, and the line does not say "nothing written".
	put(t, dir, "AGENTS.md", "<!-- BEGIN JITSO block=1 added=new: x -->\nno end\n")
	got := run(dir)
	want(t, got, Fail, "instructions", "AGENTS.md has a broken JITSO block (a BEGIN without an END at line 1)")
	if strings.Contains(got.Detail, "nothing written") {
		t.Errorf("detail = %q", got.Detail)
	}

	// CLAUDE.md is read instead of AGENTS.md: it needs the block or the import.
	put(t, dir, "AGENTS.md", block(t, 1))
	put(t, dir, "CLAUDE.md", "# Claude\n")
	want(t, run(dir), Warn, "instructions", "CLAUDE.md has no JITSO block and does not import @AGENTS.md")
	put(t, dir, "CLAUDE.md", block(t, 1))
	want(t, run(dir), OK, "instructions", "block 1 in AGENTS.md, CLAUDE.md")
	put(t, dir, "CLAUDE.md", "@AGENTS.md\n")
	want(t, run(dir), OK, "instructions", "block 1 in AGENTS.md")
	if d := run(dir).Detail; strings.Contains(d, "CLAUDE.md") {
		t.Errorf("an importing CLAUDE.md is not listed: %q", d)
	}

	// CLAUDE.local.md without the import hides AGENTS.md from Claude Code.
	os.Remove(filepath.Join(dir, "CLAUDE.md"))
	put(t, dir, "CLAUDE.local.md", "# mine\n")
	want(t, run(dir), Warn, "instructions", "CLAUDE.local.md stops Claude Code from reading AGENTS.md")
}

func TestInstructionsUser(t *testing.T) {
	home := t.TempDir()
	p := Ports{Home: home}.withDefaults()

	want(t, checkInstructionsUser(p), OK, "instructions-user", "not set up (optional)")

	put(t, home, ".claude/CLAUDE.md", "# me\n")
	want(t, checkInstructionsUser(p), OK, "instructions-user", "not set up (optional)")

	put(t, home, ".claude/CLAUDE.md", block(t, 1))
	put(t, home, ".codex/config.toml", "x")
	put(t, home, ".codex/AGENTS.md", block(t, 1))
	want(t, checkInstructionsUser(p), OK, "instructions-user", "block 1 in ~/.claude/CLAUDE.md, ~/.codex/AGENTS.md")

	put(t, home, ".codex/AGENTS.md", block(t, 0))
	want(t, checkInstructionsUser(p), Warn, "instructions-user", "block 0 in ~/.codex/AGENTS.md is older than block 1")

	put(t, home, ".claude/CLAUDE.md", "<!-- END JITSO block=1 -->\n")
	want(t, checkInstructionsUser(p), Fail, "instructions-user", "~/.claude/CLAUDE.md has a broken JITSO block (an END without a BEGIN at line 1)")

	want(t, checkInstructionsUser(Ports{}.withDefaults()), Warn, "instructions-user", "HOME is not set")
}
