package dispatch

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/fail"
)

func TestCheckRequest(t *testing.T) {
	tests := []struct {
		name, in, want string
		code           int
	}{
		{"plain", "explain the sync", "explain the sync", 0},
		{"trimmed, inner spaces kept", "  a  b \n", "a  b", 0},
		{"spanish", "prueba la historia: ñandú", "prueba la historia: ñandú", 0},
		{"empty", "", "", fail.CodeRejected},
		{"blank", " \t\n\u00a0", "", fail.CodeRejected},
		{"dash", "-rf now", "", fail.CodeRejected},
		{"long flag", "--dangerously-skip-permissions please", "", fail.CodeRejected},
		{"dash after padding", "  --evil", "", fail.CodeRejected},
		{"dash inside is fine", "a -b", "a -b", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CheckRequest(tt.in)
			if fail.Code(err) != tt.code || got != tt.want {
				t.Errorf("CheckRequest(%q) = %q, %v (exit %d), want %q (exit %d)", tt.in, got, err, fail.Code(err), tt.want, tt.code)
			}
			if tt.code != 0 && !strings.HasPrefix(fail.Text(err), "JITSO REJECTED: ") {
				t.Errorf("rejection text %q", fail.Text(err))
			}
		})
	}
}

func TestRenderPrompt(t *testing.T) {
	parts := PromptParts{Catalog: "- a: {{LAUNCHER}}", Launcher: "/opt/jitso", BriefsDir: "/h/briefs", Worktree: "path:/p"}
	tests := []struct{ name, template, want string }{
		{"all four", "{{CATALOG}}|{{LAUNCHER}} launch|{{BRIEFS_DIR}}|{{WORKTREE}}", "- a: {{LAUNCHER}}|/opt/jitso launch|/h/briefs|path:/p"},
		{"unknown stays", "{{OTHER}} {{BRIEFS_DIR}}", "{{OTHER}} /h/briefs"},
		{"repeated", "{{BRIEFS_DIR}} {{BRIEFS_DIR}}", "/h/briefs /h/briefs"},
		{"not a placeholder", "{ {LAUNCHER} } {{ LAUNCHER }}", "{ {LAUNCHER} } {{ LAUNCHER }}"},
		{"no placeholder", "plain", "plain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RenderPrompt(tt.template, parts); got != tt.want {
				t.Errorf("RenderPrompt = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSettings(t *testing.T) {
	got, err := Settings("/opt/it's/jitso")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"hooks":{"PreToolUse":[{"matcher":"Bash|Write","hooks":[{"type":"command","command":"'/opt/it'\\''s/jitso' hook dispatch-guard"}]}]}}`
	if got != want {
		t.Errorf("Settings =\n%s\nwant\n%s", got, want)
	}
	var back map[string]any
	if err := json.Unmarshal([]byte(got), &back); err != nil {
		t.Errorf("not JSON: %v", err)
	}
	if _, err := Settings("/bad\x00path"); err == nil {
		t.Error("a NUL byte in the launcher must fail")
	}
}

func TestBuildPlan(t *testing.T) {
	plan, err := BuildPlan(PlanInput{Request: "find the hook", Template: "P {{BRIEFS_DIR}} {{WORKTREE}}", Summary: "S", Launcher: "/opt/jitso", DispatchDir: "/h/.jitso/dispatch-cwd", Primary: "/p"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Dir != "/h/.jitso/dispatch-cwd" {
		t.Errorf("dir %q", plan.Dir)
	}
	settings, _ := Settings("/opt/jitso")
	want := []string{
		"claude", "--model", "claude-haiku-5-5", "--effort", "medium", "--setting-sources", "project",
		"--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--tools=Bash,Write",
		"--settings", settings, "--system-prompt", "P /h/.jitso/dispatch-cwd/briefs path:/p", "find the hook",
	}
	if !reflect.DeepEqual(plan.Argv, want) {
		t.Errorf("argv\n%q\nwant\n%q", plan.Argv, want)
	}
	for _, bad := range []string{"--dangerously-skip-permissions", "--allow-dangerously-skip-permissions", "--permission-mode", "-p", "--print", "--tools"} {
		if slices.Contains(plan.Argv, bad) {
			t.Errorf("argv carries %s", bad)
		}
	}
	if _, err := BuildPlan(PlanInput{Launcher: "/bad\x00"}); err == nil {
		t.Error("a launcher with a NUL byte must fail")
	}
}

func TestHandoffEnv(t *testing.T) {
	tests := []struct {
		name    string
		environ []string
		want    []string
	}{
		{"none", nil, []string{"JITSO_PROJECT=/p"}},
		{"added last", []string{"A=1", "B=2"}, []string{"A=1", "B=2", "JITSO_PROJECT=/p"}},
		{"inherited value is replaced", []string{"JITSO_PROJECT=/other", "A=1"}, []string{"A=1", "JITSO_PROJECT=/p"}},
		{"similar name is kept", []string{"JITSO_PROJECT_X=1", "XJITSO_PROJECT=2"}, []string{"JITSO_PROJECT_X=1", "XJITSO_PROJECT=2", "JITSO_PROJECT=/p"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := slices.Clone(tt.environ)
			if got := HandoffEnv(tt.environ, "/p"); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("HandoffEnv = %q, want %q", got, tt.want)
			}
			if !reflect.DeepEqual(in, tt.environ) {
				t.Errorf("the input slice was changed")
			}
		})
	}
}

type spawnCall struct {
	argv []string
	dir  string
	env  []string
}

func TestOpen(t *testing.T) {
	setup := func(t *testing.T, prompt *string) Options {
		t.Helper()
		root, home := t.TempDir(), t.TempDir()
		if prompt != nil {
			dir := filepath.Join(root, "jitso", "dispatcher")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte(*prompt), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return Options{Request: "do it", Root: root, Home: home, Summary: "- x", Launcher: "/opt/jitso", Environ: []string{"A=1"}}
	}
	prompt := "{{CATALOG}} {{LAUNCHER}} {{BRIEFS_DIR}}"

	t.Run("runs claude in the dispatcher folder and creates the briefs folder", func(t *testing.T) {
		o := setup(t, &prompt)
		var call spawnCall
		o.Spawn = func(argv []string, dir string, env []string) (int, error) {
			call = spawnCall{argv, dir, env}
			return 0, nil
		}
		if err := Open(o); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(o.Home, ".jitso", "dispatch-cwd")
		if call.dir != dir {
			t.Errorf("dir %q, want %q", call.dir, dir)
		}
		if info, err := os.Stat(filepath.Join(dir, "briefs")); err != nil || !info.IsDir() {
			t.Errorf("briefs folder: %v", err)
		}
		if !reflect.DeepEqual(call.env, []string{"A=1", "JITSO_PROJECT=" + o.Root}) {
			t.Errorf("env %q", call.env)
		}
		if got := call.argv[slices.Index(call.argv, "--system-prompt")+1]; got != "- x /opt/jitso "+filepath.Join(dir, "briefs") {
			t.Errorf("system prompt %q", got)
		}
		if call.argv[len(call.argv)-1] != "do it" {
			t.Errorf("the request is not the last argument: %q", call.argv)
		}
	})

	t.Run("claude's exit status is the exit status", func(t *testing.T) {
		for status, code := range map[int]int{0: 0, 7: 7, 130: 130, -1: 1} {
			o := setup(t, &prompt)
			o.Spawn = func([]string, string, []string) (int, error) { return status, nil }
			err := Open(o)
			if fail.Code(err) != code || fail.Text(err) != "" {
				t.Errorf("status %d: exit %d text %q, want exit %d and no text", status, fail.Code(err), fail.Text(err), code)
			}
		}
	})

	t.Run("claude that cannot start is exit 3 with one line", func(t *testing.T) {
		o := setup(t, &prompt)
		o.Spawn = func([]string, string, []string) (int, error) { return 0, errors.New("not found") }
		err := Open(o)
		if fail.Code(err) != fail.CodeEnv || fail.Text(err) != "jitso: could not start claude (is it on PATH?)" {
			t.Errorf("exit %d text %q", fail.Code(err), fail.Text(err))
		}
	})

	for name, mutate := range map[string]func(*Options){
		"a missing prompt file": func(o *Options) { os.RemoveAll(filepath.Join(o.Root, "jitso")) },
		"no HOME":               func(o *Options) { o.Home = "" },
	} {
		t.Run("fails with "+name+" before claude starts", func(t *testing.T) {
			o := setup(t, &prompt)
			started := false
			o.Spawn = func([]string, string, []string) (int, error) { started = true; return 0, nil }
			mutate(&o)
			err := Open(o)
			if fail.Code(err) != fail.CodeEnv || started {
				t.Errorf("exit %d, started %v, err %v", fail.Code(err), started, err)
			}
		})
	}
}

func TestBuildPlanLauncherWithSpace(t *testing.T) {
	in := PlanInput{Request: "r", Template: "{{LAUNCHER}} launch --build <id>", Summary: "S", DispatchDir: "/h/.jitso/dispatch-cwd", Primary: "/p"}
	in.Launcher = "/opt/my tools/jitso"
	plan, err := BuildPlan(in)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.Index(plan.Argv, "--system-prompt")
	if i < 0 || plan.Argv[i+1] != "'/opt/my tools/jitso' launch --build <id>" {
		t.Errorf("the prompt must quote a launcher with a space, got %q", plan.Argv)
	}
	for _, bad := range []string{"/opt/it's/jitso", "/opt/a$b/jitso", "/opt/two  spaces/jitso", "/opt/trailing /jitso x ", "/opt/a;b/jitso", "/opt/tab\there/jitso"} {
		in.Launcher = bad
		if _, err := BuildPlan(in); fail.Code(err) != fail.CodeEnv {
			t.Errorf("launcher %q: got %v (exit %d), want an environment error", bad, err, fail.Code(err))
		}
	}
}

// The launch line names the primary worktree, so the primary must pass the
// --worktree allowlist: a path the launcher would reject is an environment
// error, not a line that could never launch (defect N-23).
func TestBuildPlanPrimaryWorktree(t *testing.T) {
	in := PlanInput{Request: "r", Template: "{{LAUNCHER}} launch --worktree {{WORKTREE}}", Summary: "S", Launcher: "/opt/jitso", DispatchDir: "/h/.jitso/dispatch-cwd"}
	in.Primary = "/work/my-repo_1.x"
	plan, err := BuildPlan(in)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.Index(plan.Argv, "--system-prompt")
	if i < 0 || plan.Argv[i+1] != "/opt/jitso launch --worktree path:/work/my-repo_1.x" {
		t.Errorf("the prompt must carry path:<primary>, got %q", plan.Argv)
	}
	for _, bad := range []string{"/a b/p", "/a@b/p", "/a+b/p", "/a,b/p", "/a%b/p", "/a$b/p", "/a/../p", "/a'b/p"} {
		in.Primary = bad
		_, err := BuildPlan(in)
		want := "the dispatcher cannot name the primary worktree path:" + bad + ": --worktree allows only letters, digits and . _ : / -"
		if fail.Code(err) != fail.CodeEnv || err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("primary %q: got %v (exit %d), want an environment error %q", bad, err, fail.Code(err), want)
		}
	}
}
