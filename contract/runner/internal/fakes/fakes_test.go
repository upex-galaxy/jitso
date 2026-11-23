package fakes

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"jitso-contract-runner/internal/expect"
	"jitso-contract-runner/internal/jsonx"
)

type orcaRig struct {
	t   *testing.T
	env map[string]string
}

func newRig(t *testing.T, scenarioJSON string) *orcaRig {
	t.Helper()
	dir := t.TempDir()
	var tree any
	if scenarioJSON != "" {
		var err error
		if tree, err = jsonx.Decode([]byte(scenarioJSON)); err != nil {
			t.Fatal(err)
		}
	}
	sc, err := ParseScenario(tree, "/proj")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(sc)
	path := filepath.Join(dir, "scenario.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return &orcaRig{t: t, env: map[string]string{EnvScenario: path, EnvOrcaLog: filepath.Join(dir, "log.jsonl")}}
}

// call runs one fake orca call and returns its exit code and decoded answer.
func (r *orcaRig) call(args ...string) (int, map[string]any) {
	r.t.Helper()
	var out, errb bytes.Buffer
	code := Orca(append(args, "--json"), func(k string) string { return r.env[k] }, &out, &errb)
	var msg map[string]any
	if err := json.Unmarshal(out.Bytes(), &msg); err != nil {
		r.t.Fatalf("orca %v: stdout is not JSON: %q (stderr %q)", args, out.String(), errb.String())
	}
	return code, msg
}

func TestScenarioDefaults(t *testing.T) {
	sc, err := ParseScenario(nil, "/proj")
	if err != nil {
		t.Fatal(err)
	}
	if sc.Handle != "term_fake" || sc.Cwd == nil || *sc.Cwd != "/proj" || !reflect.DeepEqual(sc.Screens, []string{DefaultScreen}) || sc.Listed != nil || sc.WaitTimesOut {
		t.Errorf("defaults: %+v", sc)
	}
	tree, _ := jsonx.Decode([]byte(`{"cwd":null,"listed":[]}`))
	sc, _ = ParseScenario(tree, "/proj")
	if sc.Cwd != nil || sc.Listed == nil || len(sc.Listed) != 0 {
		t.Errorf("cwd null omits worktreeId and listed [] stays empty: %+v", sc)
	}
}

func TestFakeOrcaDefaultSession(t *testing.T) {
	r := newRig(t, "")
	code, msg := r.call("terminal", "create", "--title", "t", "--command", "c")
	want := map[string]any{"ok": true, "result": map[string]any{"terminal": map[string]any{"handle": "term_fake", "worktreeId": "fake-uuid::/proj"}}}
	if code != 0 || !reflect.DeepEqual(msg, want) {
		t.Errorf("create: %d %v", code, msg)
	}
	if code, msg := r.call("terminal", "wait", "--terminal", "term_fake"); code != 0 || msg["ok"] != true {
		t.Errorf("wait: %d %v", code, msg)
	}
	_, msg = r.call("terminal", "read", "--terminal", "term_fake", "--screen")
	tail := msg["result"].(map[string]any)["terminal"].(map[string]any)["tail"]
	if !reflect.DeepEqual(tail, []any{"❯ ", " medium · /effort", " bypass permissions on"}) {
		t.Errorf("read tail: %v", tail)
	}
	_, msg = r.call("terminal", "list")
	if !reflect.DeepEqual(msg["result"], map[string]any{"terminals": []any{map[string]any{"handle": "term_fake"}}}) {
		t.Errorf("list before close: %v", msg)
	}
	r.call("terminal", "close", "--terminal", "term_fake")
	_, msg = r.call("terminal", "list")
	if !reflect.DeepEqual(msg["result"], map[string]any{"terminals": []any{}}) {
		t.Errorf("list after close: %v", msg)
	}
	_, msg = r.call("orchestration", "worker-start", "--run", "r")
	if !reflect.DeepEqual(msg["result"], map[string]any{"state": "ready"}) {
		t.Errorf("other verb: %v", msg)
	}
	calls, err := expect.OrcaCalls(r.env[EnvOrcaLog])
	if err != nil || len(calls) != 7 || calls[0][len(calls[0])-1] == "--json" {
		t.Errorf("log: %v %v (trailing --json must be stripped)", calls, err)
	}
}

func TestFakeOrcaScenario(t *testing.T) {
	r := newRig(t, `{"handle":"term_x","cwd":null,"screens":["one","two"],"wait_times_out":true,
		"fail_on":["terminal send"],"errors":{"terminal close":{"code":"gone","message":"no such terminal"}},
		"listed":["term_a","term_b"],"responses":{"orchestration run-get":{"run":{"id":"R1"}}}}`)
	_, msg := r.call("terminal", "create")
	if !reflect.DeepEqual(msg["result"], map[string]any{"terminal": map[string]any{"handle": "term_x"}}) {
		t.Errorf("create with cwd null: %v", msg)
	}
	if code, msg := r.call("terminal", "wait"); code != 1 || !reflect.DeepEqual(msg["error"], map[string]any{"code": "fake_failure", "message": "terminal wait failed"}) {
		t.Errorf("wait_times_out: %d %v", code, msg)
	}
	var screens []any
	for i := 0; i < 3; i++ {
		_, msg := r.call("terminal", "read")
		screens = append(screens, msg["result"].(map[string]any)["terminal"].(map[string]any)["tail"].([]any)[0])
	}
	if !reflect.DeepEqual(screens, []any{"one", "two", "two"}) {
		t.Errorf("screens advance and the last repeats: %v", screens)
	}
	if code, _ := r.call("terminal", "send", "--text", "x"); code != 1 {
		t.Error("fail_on must fail the verb")
	}
	if code, msg := r.call("terminal", "close", "--terminal", "term_a"); code != 1 || !reflect.DeepEqual(msg["error"], map[string]any{"code": "gone", "message": "no such terminal"}) {
		t.Errorf("errors override: %d %v", code, msg)
	}
	_, msg = r.call("orchestration", "run-get")
	if !reflect.DeepEqual(msg["result"], map[string]any{"run": map[string]any{"id": "R1"}}) {
		t.Errorf("responses: %v", msg)
	}
	_, msg = r.call("terminal", "list")
	if !reflect.DeepEqual(msg["result"], map[string]any{"terminals": []any{map[string]any{"handle": "term_b"}}}) {
		t.Errorf("listed minus the closed handle: %v", msg)
	}
}

func TestFakeClaude(t *testing.T) {
	dump := filepath.Join(t.TempDir(), "dump.json")
	var errb bytes.Buffer
	code := Claude([]string{"--model", "m", "it's"}, []string{EnvClaudeDump + "=" + dump, "HOME=/h"}, &errb)
	if code != 0 || errb.Len() != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
	var d expect.ClaudeDump
	data, _ := os.ReadFile(dump)
	if err := json.Unmarshal(data, &d); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	real, _ := filepath.EvalSymlinks(wd)
	if !reflect.DeepEqual(d.Argv, []string{"--model", "m", "it's"}) || d.Env["HOME"] != "/h" || d.Cwd != real || d.Program != "claude" {
		t.Errorf("dump: %+v", d)
	}
	if code := Claude(nil, nil, &errb); code != 1 {
		t.Error("without the dump variable the fake must refuse")
	}
}

func TestFakeHarnessRecordsItsProgram(t *testing.T) {
	dump := filepath.Join(t.TempDir(), "dump.json")
	var errb bytes.Buffer
	if code := Harness("codex", []string{"-m", "x"}, []string{EnvClaudeDump + "=" + dump}, &errb); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
	var d expect.ClaudeDump
	data, _ := os.ReadFile(dump)
	if err := json.Unmarshal(data, &d); err != nil {
		t.Fatal(err)
	}
	if d.Program != "codex" || !reflect.DeepEqual(d.Argv, []string{"-m", "x"}) {
		t.Errorf("dump: %+v", d)
	}
}

func TestFakeOrcaCloseKeepsListed(t *testing.T) {
	r := newRig(t, `{"close_keeps_listed":true}`)
	r.call("terminal", "close", "--terminal", "term_fake")
	_, msg := r.call("terminal", "list")
	if !reflect.DeepEqual(msg["result"], map[string]any{"terminals": []any{map[string]any{"handle": "term_fake"}}}) {
		t.Errorf("close_keeps_listed: %v", msg)
	}
}

func TestFakeClaudeExitKnob(t *testing.T) {
	dir := t.TempDir()
	scenario := filepath.Join(dir, "scenario.json")
	data, _ := json.Marshal(Scenario{Claude: ClaudeScenario{Exit: 7}})
	if err := os.WriteFile(scenario, data, 0o644); err != nil {
		t.Fatal(err)
	}
	dump := filepath.Join(dir, "dump.json")
	var errb bytes.Buffer
	code := Claude([]string{"x"}, []string{EnvClaudeDump + "=" + dump, EnvScenario + "=" + scenario}, &errb)
	if code != 7 || errb.Len() != 0 {
		t.Errorf("exit %d, stderr %q; want 7 and nothing", code, errb.String())
	}
	if _, err := os.Stat(dump); err != nil {
		t.Error("the fake writes its dump before it exits with the knob")
	}
}

func TestFakeOrcaWaitBlocked(t *testing.T) {
	r := newRig(t, `{"wait_blocked":["agent-trust-workspace"]}`)
	code, msg := r.call("terminal", "wait")
	wait := msg["result"].(map[string]any)["wait"].(map[string]any)
	if code != 1 || msg["ok"] != true || wait["satisfied"] != false || wait["blockedReason"] != "agent-trust-workspace" {
		t.Errorf("blocked wait: %d %v", code, msg)
	}
	code, msg = r.call("terminal", "wait")
	wait = msg["result"].(map[string]any)["wait"].(map[string]any)
	if code != 0 || wait["satisfied"] != true || wait["blockedReason"] != nil {
		t.Errorf("wait past the list is idle: %d %v", code, msg)
	}
}
