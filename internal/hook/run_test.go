package hook

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// gitProject is a real git checkout (hooks resolve the project with git) with
// a jitso catalog, plus a HOME folder.
func gitProject(t *testing.T) (project, home string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project, home = filepath.Join(root, "project"), filepath.Join(root, "home")
	for _, d := range []string{filepath.Join(project, "jitso"), home} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(project, "jitso", "jitso.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", project, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git is not usable here: %v %s", err, out)
	}
	return project, home
}

func runWith(run func(Deps), stdin string, env map[string]string, self string) string {
	var out bytes.Buffer
	run(Deps{
		Stdin:      strings.NewReader(stdin),
		Stdout:     &out,
		Getenv:     func(k string) string { return env[k] },
		Getwd:      func() (string, error) { return env["PWD"], nil },
		Executable: func() (string, error) { return self, nil },
		Now:        func() time.Time { return time.Date(2026, 11, 13, 14, 30, 0, 0, time.UTC) },
	})
	return out.String()
}

func TestRunGuard(t *testing.T) {
	project, _ := gitProject(t)
	self := filepath.Join(t.TempDir(), "jitso")
	if err := os.WriteFile(self, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	self, _ = filepath.EvalSymlinks(self)
	env := map[string]string{"CLAUDE_PROJECT_DIR": project}
	selfJSON := strings.ReplaceAll(self, `\`, `\\`) // the path sits inside a JSON string
	deny := `{"tool_name":"Bash","tool_input":{"command":"orca orchestration worker-start --run r1"}}`

	tests := []struct {
		name, stdin string
		env         map[string]string
		wantDeny    string
	}{
		{"denies a direct worker-start", deny, env, "(denied: orchestration worker-start)"},
		{"names the launcher of a project without a jitso script", deny, env, "Use: jitso launch --build"},
		{"lets ls pass", `{"tool_name":"Bash","tool_input":{"command":"ls"}}`, env, ""},
		{"lets the own launcher pass", `{"tool_name":"Bash","tool_input":{"command":"` + selfJSON + ` launch --build p"}}`, env, ""},
		{"lets the own setup pass", `{"tool_name":"Bash","tool_input":{"command":"` + selfJSON + ` setup hook --remove --yes"}}`, env, ""},
		{"lets a build save pass", `{"tool_name":"Bash","tool_input":{"command":"jitso build save x"}}`, env, ""},
		{"denies a harness session", `{"tool_name":"Bash","tool_input":{"command":"claude -p hi"}}`, env, "(denied: claude)"},
		{"lets doctor pass", `{"tool_name":"Bash","tool_input":{"command":"jitso doctor"}}`, env, ""},
		{"fails open on bad JSON", `not json`, env, ""},
		{"fails open on an empty input", ``, env, ""},
		{"fails open on a non-object", `[]`, env, ""},
		{"does nothing without a catalog", deny, map[string]string{"CLAUDE_PROJECT_DIR": t.TempDir()}, ""},
		{"finds the project from the cwd", deny, map[string]string{"PWD": project}, "(denied: orchestration worker-start)"},
		{"does nothing outside git without CLAUDE_PROJECT_DIR", deny, map[string]string{"PWD": t.TempDir()}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runWith(RunGuard, tt.stdin, tt.env, self)
			if tt.wantDeny == "" {
				if got != "" {
					t.Errorf("want silence, got %q", got)
				}
				return
			}
			if !strings.Contains(got, `"permissionDecision":"deny"`) || !strings.Contains(got, tt.wantDeny) {
				t.Errorf("got %q, want a deny containing %q", got, tt.wantDeny)
			}
		})
	}
}

func TestRunGuardNamesTheBunScript(t *testing.T) {
	project, _ := gitProject(t)
	if err := os.WriteFile(filepath.Join(project, "package.json"), []byte(`{"scripts":{"jitso":"x"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := runWith(RunGuard, `{"tool_name":"Bash","tool_input":{"command":"orca agent launch"}}`, map[string]string{"CLAUDE_PROJECT_DIR": project}, "/x/jitso")
	if !strings.Contains(got, "Use: bun run jitso launch --build") || !strings.Contains(got, "(denied: agent launch)") {
		t.Errorf("got %q", got)
	}
}

func TestRunGuardSessionEnd(t *testing.T) {
	project, _ := gitProject(t)
	state := filepath.Join(project, ".jitso")
	if err := os.MkdirAll(state, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(state, "launches.jsonl")
	if err := os.WriteFile(file, []byte(launchLine+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"CLAUDE_PROJECT_DIR": project}
	input := `{"hook_event_name":"SessionEnd","session_id":"sess-1","reason":"other"}`
	for i := range 2 {
		if got := runWith(RunGuard, input, env, "/x/jitso"); got != "" {
			t.Fatalf("run %d printed %q", i, got)
		}
	}
	data, _ := os.ReadFile(file)
	if n := strings.Count(string(data), `"event":"end"`); n != 1 {
		t.Errorf("%d end events, want 1:\n%s", n, data)
	}
	if !strings.Contains(string(data), `"source":"session_end_hook"`) {
		t.Errorf("end event lacks its source:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(state, "launches.lock")); !os.IsNotExist(err) {
		t.Errorf("lock left behind: %v", err)
	}
}
