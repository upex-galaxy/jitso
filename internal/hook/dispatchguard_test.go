package hook

import (
	"strings"
	"testing"
)

func testGuard(symlinks ...string) DispatchGuard {
	links := map[string]bool{}
	for _, l := range symlinks {
		links[l] = true
	}
	return DispatchGuard{
		DispatchDir: "/h/.jitso/dispatch-cwd",
		Launcher:    "/opt/jitso",
		IsOwn:       isOwnAt("/opt/jitso"),
		Real:        func(p string) string { return p },
		IsSymlink:   func(p string) bool { return links[p] },
		Primary:     "/p",
	}
}

func payload(tool string, input map[string]any, cwd any) map[string]any {
	return map[string]any{"tool_name": tool, "tool_input": input, "cwd": cwd}
}

// req is every flag a launch line carries, in the order the template writes them.
const req = "--build p --brief /h/b/a.md --mode direct --worktree path:/p"

func TestDispatchGuardCommands(t *testing.T) {
	const launch = "/opt/jitso launch "
	tests := []struct {
		name, command string
		problem       string // "" when allowed, else the exact reason
	}{
		{"all four flags", launch + req, ""},
		{"any order", launch + "--worktree path:/p --mode direct --brief /h/b/a.md --build p", ""},
		{"all flags", launch + req + " --field a=b --field c=d --mcp-exclude m --mcp-exclude n --dry-run --dry-run", ""},
		{"optional flags first", launch + "--field a=b --dry-run " + req, ""},
		{"quoted brief with space", launch + "--build p --brief '/h/my dir/a.md' --mode direct --worktree path:/p", ""},

		{"bare launch", "/opt/jitso launch", "--build is required"},
		{"no build", launch + "--brief b --mode direct --worktree path:/p", "--build is required"},
		{"no brief", launch + "--build p --mode direct --worktree path:/p", "--brief is required"},
		{"no mode", launch + "--build p --brief b --worktree path:/p", "--mode is required"},
		{"no worktree", launch + "--build p --brief b --mode direct", "--worktree is required"},
		{"only optional flags", launch + "--field a=b --dry-run", "--build is required"},
		{"the first missing flag is named", launch + "--mode direct", "--build is required"},
		{"build twice", launch + req + " --build q", "--build is given more than once"},
		{"brief twice", launch + req + " --brief /h/b/b.md", "--brief is given more than once"},
		{"mode twice", launch + req + " --mode direct", "--mode is given more than once"},
		{"worktree twice", launch + req + " --worktree path:/p", "--worktree is given more than once"},
		{"worktree is the primary", launch + req, ""},
		{"worktree by name", launch + "--build p --brief b --mode direct --worktree wt-1", "--worktree must be path:/p"},
		{"worktree of another folder", launch + "--build p --brief b --mode direct --worktree path:/elsewhere", "--worktree must be path:/p"},
		{"worktree below the primary", launch + "--build p --brief b --mode direct --worktree path:/p/sub", "--worktree must be path:/p"},
		{"worktree as an id", launch + "--build p --brief b --mode direct --worktree id:fake-uuid::/p", "--worktree must be path:/p"},
		{"worktree with a trailing slash", launch + "--build p --brief b --mode direct --worktree path:/p/", "--worktree must be path:/p"},
		{"worktree twice, one wrong", launch + "--build p --brief b --mode direct --worktree path:/p --worktree path:/q", "--worktree is given more than once"},
		{"quoted worktree", launch + "--build p --brief b --mode direct --worktree 'path:/p'", "characters outside the plain set"},

		{"other program", "ls", "only"},
		{"bun run form", "bun run jitso launch " + req, "only"},
		{"program in front", "python3 /opt/jitso launch " + req, "only"},
		{"foreign launcher", "/tmp/evil/jitso launch " + req, "only"},
		{"relative launcher", "jitso launch " + req, "only"},
		{"env prefix", "FOO=bar /opt/jitso launch " + req, "only"},
		{"other subcommand", "/opt/jitso close L1", "only"},
		{"only the launcher", "/opt/jitso", "only"},
		{"semicolon", launch + req + "; ls", "characters outside the plain set"},
		{"substitution", launch + req + " --field $(whoami)", "characters outside the plain set"},
		{"backtick", launch + req + " --field `whoami`", "characters outside the plain set"},
		{"pipe", launch + req + " | cat", "characters outside the plain set"},
		{"and", launch + req + " && ls", "characters outside the plain set"},
		{"redirect", launch + req + " > /tmp/x", "characters outside the plain set"},
		{"newline", launch + req + "\nls", "characters outside the plain set"},
		{"quote", launch + "--build 'p' --brief b --mode direct --worktree path:/p", "characters outside the plain set"},
		{"double space", launch + "--build p  --dry-run", "characters outside the plain set"},
		{"fleet", launch + "--build p --brief b --mode fleet --worktree path:/p", "direct mode only"},
		{"upper-case mode", launch + "--build p --brief b --mode DIRECT --worktree path:/p", "direct mode only"},
		{"unknown flag", launch + req + " --model m", `flag "--model" is not allowed`},
		{"stray word", launch + req + " stray", `flag "stray" is not allowed`},
		{"flag without value", launch + "--build p --mode", "--mode needs a value"},
		{"value that is a flag", launch + "--build --brief x", "--build needs a value"},
		{"quoted value of another flag", launch + req + " --field 'a=b c'", "a quoted word is allowed only"},
		{"old preset flag", launch + "--preset p --brief b --mode direct", `flag "--preset" is not allowed`},
		{"quoted flag", launch + "'--dry-run'", "characters outside the plain set"},
		{"quoted subcommand", "/opt/jitso 'launch x'", "only"},
	}
	g := testGuard()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			problem := g.Problem(payload("Bash", map[string]any{"command": tt.command}, g.DispatchDir))
			if tt.problem == "" && problem != "" || !strings.Contains(problem, tt.problem) {
				t.Errorf("command %q: problem %q, want %q", tt.command, problem, tt.problem)
			}
		})
	}
}

func TestDispatchGuardQuotedLauncher(t *testing.T) {
	g := testGuard()
	g.Launcher = "/opt/my tools/jitso"
	g.IsOwn = isOwnAt("/opt/my tools/jitso")
	for command, allowed := range map[string]bool{
		"'/opt/my tools/jitso' launch " + req:          true,
		"'/opt/my tools/jitso' launch":                 false,
		"/opt/my tools/jitso launch " + req:            false,
		"'/opt/my tools/other' launch " + req:          false,
		"'/opt/my tools/jitso' close L1":               false,
		"'/opt/my tools/jitso' launch " + req + "; ls": false,
	} {
		problem := g.Problem(payload("Bash", map[string]any{"command": command}, g.DispatchDir))
		if (problem == "") != allowed {
			t.Errorf("command %q: problem %q, allowed want %v", command, problem, allowed)
		}
	}
}

func TestDispatchGuardWrites(t *testing.T) {
	const briefs = "/h/.jitso/dispatch-cwd/briefs"
	tests := []struct {
		name     string
		path     any
		symlinks []string
		allowed  bool
	}{
		{"a brief", briefs + "/explore-jira.md", nil, true},
		{"underscore dot dash and capital", briefs + "/Story_PRJ-120.v2.md", nil, true},
		{"upper-case extension", briefs + "/x.MD", nil, true},
		{"not markdown", briefs + "/run.sh", nil, false},
		{"nested", briefs + "/deep/x.md", nil, false},
		{"traversal", briefs + "/../x.md", nil, false},
		{"hidden", briefs + "/.hidden.md", nil, false},
		{"space", briefs + "/my brief.md", nil, false},
		{"the briefs folder itself", briefs, nil, false},
		{"the dispatcher folder", "/h/.jitso/dispatch-cwd/x.md", nil, false},
		{"home", "/h/x.md", nil, false},
		{"relative", "briefs/x.md", nil, false},
		{"tilde", "~/.jitso/dispatch-cwd/briefs/x.md", nil, false},
		{"NUL", briefs + "/x\x00.md", nil, false},
		{"missing", nil, nil, false},
		{"not a string", 7.0, nil, false},
		{"existing symlink", briefs + "/link.md", []string{briefs + "/link.md"}, false},
		{"symlinked briefs folder", briefs + "/x.md", []string{briefs}, false},
		{"unclean path with dot segments", briefs + "/./x.md", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := testGuard(tt.symlinks...)
			problem := g.Problem(payload("Write", map[string]any{"file_path": tt.path}, g.DispatchDir))
			if (problem == "") != tt.allowed {
				t.Errorf("path %v: problem %q, allowed want %v", tt.path, problem, tt.allowed)
			}
		})
	}
}

func TestDispatchGuardSession(t *testing.T) {
	g := testGuard()
	ok := map[string]any{"command": "/opt/jitso launch " + req}
	tests := []struct {
		name  string
		input map[string]any
		want  string
	}{
		{"cwd is the dispatcher folder", payload("Bash", ok, g.DispatchDir), ""},
		{"no cwd", map[string]any{"tool_name": "Bash", "tool_input": ok}, "the session must run in"},
		{"cwd not a string", payload("Bash", ok, 5.0), "the session must run in"},
		{"cwd elsewhere", payload("Bash", ok, "/h/outside"), "the session must run in"},
		{"cwd is the briefs folder", payload("Bash", ok, g.DispatchDir+"/briefs"), "the session must run in"},
		{"no tool name", map[string]any{"tool_input": ok, "cwd": g.DispatchDir}, `the tool "undefined"`},
		{"other tool", payload("Read", map[string]any{"file_path": "/etc/passwd"}, g.DispatchDir), `the tool "Read"`},
		{"lower-case bash", payload("bash", ok, g.DispatchDir), `the tool "bash"`},
		{"tool name not a string", map[string]any{"tool_name": 5.0, "cwd": g.DispatchDir}, `the tool "unknown"`},
		{"bash without tool_input", map[string]any{"tool_name": "Bash", "cwd": g.DispatchDir}, "characters outside the plain set"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.Problem(tt.input)
			if tt.want == "" && got != "" || !strings.Contains(got, tt.want) {
				t.Errorf("Problem = %q, want it to contain %q", got, tt.want)
			}
		})
	}
}

func TestDispatchGuardRejectsThePresetFlag(t *testing.T) {
	g := testGuard()
	problem := g.Problem(payload("Bash", map[string]any{"command": "/opt/jitso launch --preset p --brief b --mode direct"}, g.DispatchDir))
	if problem != `flag "--preset" is not allowed` {
		t.Errorf("problem %q", problem)
	}
}

// A line with --worktree is denied with the reason of an unknown primary, and
// a line without it is not asked about the primary (defect N-23).
func TestDispatchGuardWorktreeNeedsThePrimary(t *testing.T) {
	g := testGuard()
	g.Primary = ""
	g.PrimaryProblem = "JITSO_PROJECT is not set, so the primary worktree is unknown"
	with := "/opt/jitso launch --build p --brief b --mode direct --worktree path:/p"
	if got := g.Problem(payload("Bash", map[string]any{"command": with}, g.DispatchDir)); got != g.PrimaryProblem {
		t.Errorf("problem %q, want %q", got, g.PrimaryProblem)
	}
	without := "/opt/jitso launch --build p --brief b --mode direct"
	if got := g.Problem(payload("Bash", map[string]any{"command": without}, g.DispatchDir)); got != "--worktree is required" {
		t.Errorf("a line without --worktree is a missing flag, not a question about the primary, got %q", got)
	}
	if got := testGuard().Problem(payload("Bash", map[string]any{"command": with + "/x"}, testGuard().DispatchDir)); got != "--worktree must be path:/p" {
		t.Errorf("problem %q", got)
	}
}

// A brief is written once: any entry already at the name (a file, a folder, a
// link) denies the Write, with the path in the reason (defect N-29, H5).
func TestDispatchGuardExistingBrief(t *testing.T) {
	const briefs = "/h/.jitso/dispatch-cwd/briefs"
	g := testGuard()
	g.Exists = func(p string) bool { return p == briefs+"/old.md" || p == briefs+"/dir.md" || p == briefs+"/link.md" }
	for name, want := range map[string]string{
		"old.md":       briefs + "/old.md already exists: write the brief to a new file name",
		"dir.md":       briefs + "/dir.md already exists: write the brief to a new file name",
		"new.md":       "",
		"old.md.again": "Write is allowed only for",
	} {
		got := g.Problem(payload("Write", map[string]any{"file_path": briefs + "/" + name}, g.DispatchDir))
		if name == "old.md.again" {
			if !strings.Contains(got, want) {
				t.Errorf("%s: problem %q", name, got)
			}
			continue
		}
		if got != want {
			t.Errorf("%s: problem %q, want %q", name, got, want)
		}
	}
	// an unclean spelling of an existing brief is the same entry
	if got := g.Problem(payload("Write", map[string]any{"file_path": briefs + "/./old.md"}, g.DispatchDir)); !strings.Contains(got, "already exists") {
		t.Errorf("problem %q", got)
	}
	// a symlink keeps its own reason
	g.IsSymlink = func(p string) bool { return p == briefs+"/link.md" }
	if got := g.Problem(payload("Write", map[string]any{"file_path": briefs + "/link.md"}, g.DispatchDir)); got != "a symlink is not a valid brief location" {
		t.Errorf("problem %q", got)
	}
}
