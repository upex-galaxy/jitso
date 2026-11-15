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
	}
}

func payload(tool string, input map[string]any, cwd any) map[string]any {
	return map[string]any{"tool_name": tool, "tool_input": input, "cwd": cwd}
}

func TestDispatchGuardCommands(t *testing.T) {
	const launch = "/opt/jitso launch "
	tests := []struct {
		name, command string
		allowed       bool
	}{
		{"bare launch", "/opt/jitso launch", true},
		{"preset brief mode", launch + "--preset p --brief /h/b/a.md --mode direct", true},
		{"all flags", launch + "--preset p --brief b --mode direct --field a=b --field c=d --worktree w --mcp-exclude m --dry-run", true},
		{"other program", "ls", false},
		{"bun run form", "bun run jitso launch --preset p", false},
		{"program in front", "python3 /opt/jitso launch --preset p", false},
		{"foreign launcher", "/tmp/evil/jitso launch --preset p", false},
		{"relative launcher", "jitso launch --preset p", false},
		{"env prefix", "FOO=bar /opt/jitso launch --preset p", false},
		{"other subcommand", "/opt/jitso close L1", false},
		{"only the launcher", "/opt/jitso", false},
		{"semicolon", launch + "--preset p; ls", false},
		{"substitution", launch + "--preset $(whoami)", false},
		{"backtick", launch + "--preset `whoami`", false},
		{"pipe", launch + "--preset p | cat", false},
		{"and", launch + "--preset p && ls", false},
		{"redirect", launch + "--preset p > /tmp/x", false},
		{"newline", launch + "--preset p\nls", false},
		{"quote", launch + "--preset 'p'", false},
		{"double space", launch + "--preset p  --dry-run", false},
		{"trailing space", launch + "--preset p ", false},
		{"fleet", launch + "--preset p --mode fleet", false},
		{"upper-case mode", launch + "--preset p --mode DIRECT", false},
		{"unknown flag", launch + "--preset p --model m", false},
		{"stray word", launch + "--preset p stray", false},
		{"flag without value", launch + "--preset p --mode", false},
		{"value that is a flag", launch + "--preset --brief x", false},
		{"empty command", "", false},
	}
	g := testGuard()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			problem := g.Problem(payload("Bash", map[string]any{"command": tt.command}, g.DispatchDir))
			if (problem == "") != tt.allowed {
				t.Errorf("command %q: problem %q, allowed want %v", tt.command, problem, tt.allowed)
			}
		})
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
	ok := map[string]any{"command": "/opt/jitso launch"}
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
