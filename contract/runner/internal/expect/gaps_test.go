package expect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Tests for the format additions G1, G2, G3, G5, G6, G7 (entries).

func TestArgvPatternsAndCount(t *testing.T) {
	dir := t.TempDir()
	dump := filepath.Join(dir, "dump.json")
	data, _ := json.Marshal(ClaudeDump{Argv: []string{
		"--dangerously-skip-permissions", "--session-id", "0f8fad5b-d9cb-469f-a165-70867728950e", "--model", "claude-haiku-5-5", "Read it.",
	}})
	write(t, dump, string(data), 0o644)
	obs := &Observation{ClaudeDump: dump}
	ok := `{"claude":{
		"argv":["--dangerously-skip-permissions","--session-id",{"regex":"^[0-9a-f-]{36}$"},"--model",{"contains":"haiku"},{"equals":"Read it."}],
		"argv_count":{"--dangerously-skip-permissions":1,"--agents":0},
		"arg_after":{"--session-id":{"regex":"^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"},"--model":"claude-haiku-5-5"}}}`
	expectFailures(t, "all hold", build(t, ok, KindExec).Eval(obs), 0)
	bad := `{"claude":{
		"argv":["--dangerously-skip-permissions"],
		"argv_count":{"--dangerously-skip-permissions":2},
		"arg_after":{"--session-id":{"regex":"^x$"},"--model":"haiku"}}}`
	expectFailures(t, "four break", build(t, bad, KindExec).Eval(obs), 4)
	// a literal string in argv is exact, never the contains shorthand
	expectFailures(t, "literal is exact", build(t, `{"claude":{"arg_after":{"--model":"haiku"}}}`, KindExec).Eval(obs), 1)

	log := filepath.Join(dir, "orca.jsonl")
	write(t, log, `{"argv":["terminal","create","--title","[jitso:L1] p"]}`+"\n", 0o644)
	orca := &Observation{OrcaLog: log}
	expectFailures(t, "orca argv takes any matcher", build(t, `{"orca_calls":{"calls":[{"at":0,"argv":["terminal","create","--title",{"contains":"[jitso:"}]}]}}`, KindRun).Eval(orca), 0)
}

func TestExitSets(t *testing.T) {
	obs := &Observation{Kind: KindRun, Exit: 3}
	expectFailures(t, "hold", build(t, `{"exit_not":0,"exit_in":[2,3]}`, KindRun).Eval(obs), 0)
	expectFailures(t, "break", build(t, `{"exit_not":3,"exit_in":[0,1]}`, KindRun).Eval(obs), 2)
	timedOut := &Observation{Kind: KindRun, TimedOut: true, Exit: -1}
	expectFailures(t, "a timeout satisfies neither", build(t, `{"exit_not":0,"exit_in":[-1]}`, KindRun).Eval(timedOut), 2)
	if _, err := Build(map[string]any{"exit_in": []any{}}, KindRun); err == nil {
		t.Error("exit_in [] must be a build error")
	}
}

func TestJSONKeySets(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.json"), `{"plan":{"description":"d","prompt":"p"},"code":{},"verifier":{}}`, 0o644)
	obs := &Observation{Project: dir}
	ok := `{"files":[{"path":"a.json","json":[
		{"path":"","keys_exact":["code","plan","verifier"]},
		{"path":"","keys_include":["plan"]},
		{"path":"plan","keys_exact":["prompt","description"]}]}]}`
	expectFailures(t, "hold", build(t, ok, KindRun).Eval(obs), 0)
	bad := `{"files":[{"path":"a.json","json":[
		{"path":"","keys_exact":["code","plan"]},
		{"path":"","keys_exact":["code","plan","verifier","extra"]},
		{"path":"","keys_include":["extra"]},
		{"path":"plan.prompt","keys_include":[]},
		{"path":"nope","keys_exact":[]}]}]}`
	expectFailures(t, "five break", build(t, bad, KindRun).Eval(obs), 5)
}

func TestFileTextAndEntries(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "card.txt"), "JITSO L1 | x\nROUTE-SCOPE: a, b\n", 0o644)
	write(t, filepath.Join(dir, "launches", "L1", "run.sh"), "x", 0o644)
	write(t, filepath.Join(dir, "launches", "L2", "run.sh"), "x", 0o644)
	if err := os.MkdirAll(filepath.Join(dir, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	obs := &Observation{Project: dir}
	ok := `{"files":[
		{"path":"card.txt","text":[{"route_scope":["a","b"]},{"line_count":2},"JITSO L1"]},
		{"path":"launches","entries":{"count":2}},
		{"path":"launches","entries":[{"equals":"L1\nL2"},{"line_count":2}]},
		{"path":"empty","entries":{"count":0}}]}`
	expectFailures(t, "hold", build(t, ok, KindRun).Eval(obs), 0)
	bad := `{"files":[
		{"path":"card.txt","text":[{"route_scope":["a"]}]},
		{"path":"launches","entries":{"count":1}},
		{"path":"card.txt","entries":{"count":0}}]}`
	expectFailures(t, "three break", build(t, bad, KindRun).Eval(obs), 3)
	for _, js := range []string{
		`{"files":[{"path":"x","exists":false,"entries":{"count":0}}]}`,
		`{"files":[{"path":"x","entries":{"count":0},"contains":["a"]}]}`,
		`{"files":[{"path":"x","entries":{"n":0}}]}`,
	} {
		v := map[string]any{}
		if err := json.Unmarshal([]byte(js), &v); err != nil {
			t.Fatal(err)
		}
		if _, err := Build(v, KindRun); err == nil {
			t.Errorf("%s: want a build error", js)
		}
	}
}

func TestRegistryCountsRawLines(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, RegistryPath), `{"event":"launch","id":"L1"}`+"\n{ this is not json\n\n"+`{"event":"end","id":"L1"}`+"\n", 0o644)
	obs := &Observation{Project: dir}
	expectFailures(t, "count and parsed lines", build(t, `{"registry":{"count":3,"events":[{"at":0,"event":"launch"},{"at":2,"event":"end"}]}}`, KindRun).Eval(obs), 0)
	fs := build(t, `{"registry":{"events":[{"at":1,"event":"launch"}]}}`, KindRun).Eval(obs)
	if len(fs) != 1 || fs[0].Expected != "a JSON line" {
		t.Errorf("only the line an event check names is parsed: %v", fs)
	}
}
