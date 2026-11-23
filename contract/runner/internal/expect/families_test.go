package expect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"jitso-contract-runner/internal/jsonx"
)

func build(t *testing.T, js, kind string) Set {
	t.Helper()
	v, err := jsonx.Decode([]byte(js))
	if err != nil {
		t.Fatal(err)
	}
	set, err := Build(v, kind)
	if err != nil {
		t.Fatalf("Build(%s): %v", js, err)
	}
	return set
}

func whats(fs []Failure) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.What
	}
	return out
}

func expectFailures(t *testing.T, name string, fs []Failure, n int) {
	t.Helper()
	if len(fs) != n {
		t.Errorf("%s: want %d failures, got %d: %v", name, n, len(fs), fs)
	}
}

func write(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func TestProcessChecks(t *testing.T) {
	obs := &Observation{Kind: KindRun, Exit: 2, Stdout: "JITSO L1\ncard", Stderr: "", Elapsed: 30 * time.Millisecond}
	expectFailures(t, "all hold", build(t, `{"exit":2,"stdout":["JITSO",{"line_count":2}],"stderr":[{"empty":true}],"elapsed_ms_lt":1000}`, KindRun).Eval(obs), 0)
	fs := build(t, `{"exit":0,"stdout":["nope"],"elapsed_ms_lt":10}`, KindRun).Eval(obs)
	expectFailures(t, "three break", fs, 3)
	if got := whats(fs); got[0] != "elapsed_ms_lt" || got[1] != "exit" || got[2] != "stdout[0] contains" {
		t.Errorf("failure names: %v", got)
	}
	timedOut := &Observation{Kind: KindRun, TimedOut: true, Exit: -1}
	if fs := build(t, `{"exit":-1}`, KindRun).Eval(timedOut); len(fs) != 1 || !strings.Contains(fs[0].Actual, "timed out") {
		t.Errorf("a timeout never satisfies exit: %v", fs)
	}
}

func TestHookChecks(t *testing.T) {
	obs := &Observation{Kind: KindHook, Decision: "deny", Reason: "JITSO owns worker launch (denied: agent launch)"}
	expectFailures(t, "deny holds", build(t, `{"decision":"deny","reason":["agent launch",{"regex":"^JITSO"}]}`, KindHook).Eval(obs), 0)
	expectFailures(t, "allow breaks", build(t, `{"decision":"allow"}`, KindHook).Eval(obs), 1)
	if _, err := Build(map[string]any{"decision": "none"}, KindRun); err == nil {
		t.Error("decision is a hook-only expectation")
	}
}

func TestFileChecks(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "run.sh"), "#!/bin/sh\nexec 'claude'\n", 0o755)
	write(t, filepath.Join(dir, "mcp.json"), `{"mcpServers":{"dbhub":{"command":"fixture-dbhub","args":["a"]}},"cap":0.5}`, 0o644)
	obs := &Observation{Project: dir}
	ok := `{"files":[
		{"path":"run.sh","executable":true,"contains":["exec 'claude'"],"not_contains":["secret"],"regex":["^#!/bin/sh"]},
		{"path":"mcp.json","executable":false,"json":[
			{"path":"mcpServers.dbhub.command","equals":"fixture-dbhub"},
			{"path":"mcpServers.dbhub.args.0","regex":"^a$"},
			{"path":"mcpServers.dbhub.env","absent":true},
			{"path":"cap","equals":0.50}]},
		{"path":` + jsonString(filepath.Join(dir, "nope")) + `,"exists":false}]}`
	expectFailures(t, "all hold", build(t, ok, KindWrite).Eval(obs), 0)
	bad := `{"files":[
		{"path":"run.sh","executable":false,"equals":"x"},
		{"path":"mcp.json","json":[{"path":"mcpServers.dbhub.env","equals":{}}]},
		{"path":"nope"}]}`
	wantBreaks := 4
	if runtime.GOOS == "windows" {
		wantBreaks = 3 // no execute bit to fail
	}
	expectFailures(t, "four break", build(t, bad, KindWrite).Eval(obs), wantBreaks)
}

func TestRegistryChecks(t *testing.T) {
	dir := t.TempDir()
	obs := &Observation{Project: dir}
	expectFailures(t, "missing registry is zero lines", build(t, `{"registry":{"count":0}}`, KindRun).Eval(obs), 0)
	write(t, filepath.Join(dir, RegistryPath),
		`{"event":"launch","id":"L20261115T005309-e8f8","mode":"direct","orca":{"terminal":"term_fake"},"budget":{"usd_list_cap":0.5},"parent":null}`+"\n\n"+
			`{"event":"end","id":"L20261115T005309-e8f8"}`+"\n", 0o644)
	ok := `{"registry":{"count":2,"events":[
		{"at":0,"event":"launch","fields":{"mode":"direct","orca.terminal":"term_fake","id":{"regex":"^L\\d{8}T\\d{6}-[0-9a-f]{4}$"},
		 "budget.usd_list_cap":0.50,"parent":null,"worktree":{"absent":true}}},
		{"at":1,"event":"end"}]}}`
	expectFailures(t, "all hold", build(t, ok, KindRun).Eval(obs), 0)
	bad := `{"registry":{"count":1,"events":[{"at":0,"event":"end","fields":{"parent":{"absent":true}}},{"at":5}]}}`
	expectFailures(t, "four break", build(t, bad, KindRun).Eval(obs), 4)
}

func TestOrcaChecks(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "orca.jsonl")
	obs := &Observation{OrcaLog: log}
	expectFailures(t, "missing log is empty", build(t, `{"orca_calls":{"empty":true,"count":0,"verbs":[]}}`, KindRun).Eval(obs), 0)
	write(t, log, `{"argv":["terminal","create","--title","[jitso:L1] p","--command","'/x/run.sh'"]}`+"\n"+`{"argv":["terminal","wait"]}`+"\n", 0o644)
	ok := `{"orca_calls":{"count":2,"verbs":["terminal create","terminal wait"],"none":["terminal send"],
		"calls":[{"at":0,"argv":["terminal","create","--title",{"regex":"^\\[jitso:L"},"--command",{"regex":"run\\.sh'$"}]}]}}`
	expectFailures(t, "all hold", build(t, ok, KindRun).Eval(obs), 0)
	bad := `{"orca_calls":{"empty":true,"verbs":["terminal wait"],"none":["terminal wait"],"calls":[{"at":1,"argv":["terminal"]},{"at":9,"argv":[]}]}}`
	expectFailures(t, "five break", build(t, bad, KindRun).Eval(obs), 5)
}

func TestClaudeChecks(t *testing.T) {
	dir := t.TempDir()
	dump := filepath.Join(dir, "dump.json")
	obs := &Observation{ClaudeDump: dump}
	expectFailures(t, "ran false without a dump", build(t, `{"claude":{"ran":false}}`, KindExec).Eval(obs), 0)
	expectFailures(t, "ran true without a dump", build(t, `{"claude":{"ran":true}}`, KindExec).Eval(obs), 1)
	data, _ := json.Marshal(ClaudeDump{
		Argv: []string{"--model", "claude-haiku-5-5", "--agents", `{"plan":{"description":"d","prompt":"p"}}`, "ROUTE-SCOPE: a\nRead it."},
		Cwd:  "/p",
		Env:  map[string]string{"CLAUDE_CODE_DISABLE_CLAUDE_MDS": "1", "PATH": "/b:/usr/bin"},
	})
	write(t, dump, string(data), 0o644)
	ok := `{"claude":{"ran":true,"argv_contains":["--agents"],"argv_not_contains":["--evil"],"last_arg":[{"route_scope":["a"]}],
		"cwd":"/p","env":{"CLAUDE_CODE_DISABLE_CLAUDE_MDS":"1","JITSO_X":{"absent":true},"PATH":{"regex":"^/b:"}},
		"arg_after":{"--model":"claude-haiku-5-5"},"json_arg_after":{"--agents":[{"path":"plan.description","regex":".+"},{"path":"code","absent":true}]}}}`
	expectFailures(t, "all hold", build(t, ok, KindExec).Eval(obs), 0)
	bad := `{"claude":{"argv":["--model"],"argv_contains":["--tools"],"cwd":"/q","env":{"CLAUDE_CODE_DISABLE_CLAUDE_MDS":{"absent":true}},
		"arg_after":{"--effort":"low"},"json_arg_after":{"--model":[{"path":"x","absent":true}]}}}`
	expectFailures(t, "six break", build(t, bad, KindExec).Eval(obs), 6)
	expectFailures(t, "ran false with a dump", build(t, `{"claude":{"ran":false}}`, KindExec).Eval(obs), 1)
	if _, err := Build(map[string]any{"claude": map[string]any{}}, KindHook); err == nil {
		t.Error("claude is not available on a hook step")
	}
}

// jsonString is s as a JSON string literal (a Windows path holds backslashes).
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
