package spec

import (
	"strings"
	"testing"
)

// Tests for the format additions G4, G7 (capture sources), G8 (claude field), G10.

func TestForEachRowsAndIDs(t *testing.T) {
	c, err := writeCase(t, "m", `{"id":"m","title":"t",
		"for_each":{"vars":["flag","value"],"rows":[["--run","{{PROJECT}}/x"],["--task","{{{{a}}"]],"ids":["run","task"]},
		"steps":[{"run":["{{flag}}","{{value}}","{{index}}"]}]}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	xs := c.Expand()
	if len(xs) != 2 || xs[0].ID != "m[run]" || xs[1].ID != "m[task]" {
		t.Fatalf("ids: %v", []string{xs[0].ID, xs[1].ID})
	}
	builtins := Vars{VarProject: "/p", VarHome: "/h", VarBin: "/b", VarJitso: "/j"}
	a0, err := xs[0].Action(0, xs[0].Bind(builtins))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(a0.Argv, " ") != "--run /p/x 0" {
		t.Errorf("row 0 argv = %q (built-ins inside a value must resolve)", a0.Argv)
	}
	a1, err := xs[1].Action(0, xs[1].Bind(builtins))
	if err != nil {
		t.Fatal(err)
	}
	if a1.Argv[1] != "{{{{a}}" {
		t.Errorf("row 1 value = %q: a value is verbatim except the built-ins", a1.Argv[1])
	}
}

func TestForEachErrors(t *testing.T) {
	cases := []struct{ name, forEach, want string }{
		{"both forms", `{"var":"a","values":["x"],"vars":["b"],"rows":[["y"]]}`, "either var and values"},
		{"short row", `{"vars":["a","b"],"rows":[["x"]]}`, "1 values for 2 vars"},
		{"duplicate var", `{"vars":["a","a"],"rows":[["x","y"]]}`, "listed twice"},
		{"reserved var", `{"vars":["WORKTREE_x"],"rows":[["x"]]}`, "not a free placeholder name"},
		{"ids length", `{"var":"a","values":["x","y"],"ids":["one"]}`, "1 ids for 2 expansions"},
		{"duplicate id", `{"var":"a","values":["x","y"],"ids":["one","one"]}`, "not a plain unique name"},
		{"bad id", `{"var":"a","values":["x"],"ids":["a b"]}`, "not a plain unique name"},
		{"no rows", `{"vars":["a"],"rows":[]}`, "at least one value"},
	}
	for _, c := range cases {
		_, err := writeCase(t, "x", `{"id":"x","title":"t","for_each":`+c.forEach+`,"steps":[{"run":[]}]}`, nil)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want an error containing %q, got %v", c.name, c.want, err)
		}
	}
}

func TestWorktreeStepAndPlaceholder(t *testing.T) {
	c, err := writeCase(t, "w", `{"id":"w","title":"t","orca":{"cwd":"{{WORKTREE_linked}}"},
		"steps":[{"git_worktree":{"name":"linked"},"expect":{"files":[{"path":"{{WORKTREE_linked}}/.git"}]}},
		         {"run":["presets","list"],"cwd":"{{WORKTREE_linked}}"}]}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Worktrees(); len(got) != 1 || got[0] != "linked" {
		t.Errorf("worktrees = %v", got)
	}
	for _, body := range []string{
		`{"id":"x","title":"t","steps":[{"git_worktree":{"name":"a b"}}]}`,
		`{"id":"x","title":"t","steps":[{"git_worktree":{"name":"a"}},{"git_worktree":{"name":"a"}}]}`,
		`{"id":"x","title":"t","steps":[{"git_worktree":{"name":"a","branch":"b"}}]}`,
		`{"id":"x","title":"t","steps":[{"git_worktree":{"name":"a"},"expect":{"exit":0}}]}`,
		`{"id":"x","title":"t","steps":[{"run":["{{WORKTREE_nope}}"]}]}`,
	} {
		if _, err := writeCase(t, "x", body, nil); err == nil {
			t.Errorf("%s: want a load error", body)
		}
	}
}

func TestCaptureSources(t *testing.T) {
	if _, err := writeCase(t, "c", `{"id":"c","title":"t","steps":[{"run":[],"capture":{
		"A":{"from":"file","path":".jitso/x","regex":"(.)"},
		"B":{"from":"orca_log","regex":"\\[jitso:(L[^\\]]+)\\]"}},"expect":{"stdout":["{{A}}{{B}}"]}}]}`, nil); err != nil {
		t.Fatal(err)
	}
	for _, capture := range []string{
		`{"from":"file","regex":"(.)"}`,
		`{"from":"stdout","path":"x","regex":"(.)"}`,
		`{"from":"registry","regex":"(.)"}`,
	} {
		if _, err := writeCase(t, "x", `{"id":"x","title":"t","steps":[{"run":[],"capture":{"A":`+capture+`}}]}`, nil); err == nil {
			t.Errorf("%s: want a load error", capture)
		}
	}
}

func TestClaudeField(t *testing.T) {
	c, err := writeCase(t, "k", `{"id":"k","title":"t","claude":{"exit":7},"steps":[{"run":[]}]}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	setup, err := c.Expand()[0].Setup(Vars{VarProject: "/p", VarHome: "/h", VarBin: "/b", VarJitso: "/j"})
	if err != nil || setup.Scenario.Claude.Exit != 7 || setup.Scenario.Claude.Missing {
		t.Errorf("claude knobs: %+v %v", setup.Scenario.Claude, err)
	}
	for _, field := range []string{`{"exit":300}`, `{"missing":true,"exit":1}`, `{"absent":true}`, `"x"`} {
		if _, err := writeCase(t, "x", `{"id":"x","title":"t","claude":`+field+`,"steps":[{"run":[]}]}`, nil); err == nil {
			t.Errorf("claude %s: want a load error", field)
		}
	}
}

func TestGitInitStep(t *testing.T) {
	good := []struct{ name, step, wantPath string }{
		{"relative", `{"git_init":{"path":"nested"},"expect":{"files":[{"path":"nested/.git"}]}}`, "nested"},
		{"nested relative", `{"git_init":{"path":"a/b/c"}}`, "a/b/c"},
		{"placeholder home", `{"git_init":{"path":"{{HOME}}/repo"},"expect":{"elapsed_ms_lt":60000,"registry":{"count":0},"orca_calls":{"empty":true}}}`, stub("/stub/home/repo")},
	}
	builtins := Vars{VarProject: stub("/stub/project"), VarHome: stub("/stub/home"), VarBin: stub("/stub/bin"), VarJitso: stub("/stub/jitso")}
	for _, g := range good {
		c, err := writeCase(t, "g", `{"id":"g","title":"t","steps":[`+g.step+`]}`, nil)
		if err != nil {
			t.Errorf("%s: %v", g.name, err)
			continue
		}
		x := c.Expand()[0]
		a, err := x.Action(0, x.Bind(builtins))
		if err != nil || a.Kind != "git_init" || a.InitPath != g.wantPath {
			t.Errorf("%s: action %+v, err %v", g.name, a, err)
		}
	}
	bad := []struct{ name, step, want string }{
		{"not an object", `{"git_init":"nested"}`, "git_init"},
		{"no path", `{"git_init":{}}`, "needs path"},
		{"path not a string", `{"git_init":{"path":3}}`, "git_init.path"},
		{"empty path", `{"git_init":{"path":""}}`, "must not be empty"},
		{"unknown key", `{"git_init":{"path":"n","branch":"b"}}`, "branch"},
		{"relative escape", `{"git_init":{"path":"../out"}}`, "leaves the project dir"},
		{"two kinds", `{"git_init":{"path":"n"},"run":[]}`, "exactly one"},
		{"step key of another kind", `{"git_init":{"path":"n"},"cwd":"."}`, "cwd"},
		{"exit expectation", `{"git_init":{"path":"n"},"expect":{"exit":0}}`, "expect.exit"},
		{"stdout expectation", `{"git_init":{"path":"n"},"expect":{"stdout":[{"empty":true}]}}`, "expect.stdout"},
		{"claude expectation", `{"git_init":{"path":"n"},"expect":{"claude":{}}}`, "expect.claude"},
	}
	for _, b := range bad {
		_, err := writeCase(t, "x", `{"id":"x","title":"t","steps":[`+b.step+`]}`, nil)
		if err == nil || !strings.Contains(err.Error(), b.want) {
			t.Errorf("%s: want a load error containing %q, got %v", b.name, b.want, err)
		}
	}
}

func stub(p string) string { return stubPath(p) }
