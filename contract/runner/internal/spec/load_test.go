package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCase lays out cases/<id>/case.json and a fixtures/default folder, and loads it.
func writeCase(t *testing.T, id, body string, extra map[string]string) (*Case, error) {
	t.Helper()
	root := t.TempDir()
	cases := filepath.Join(root, "cases")
	fixtures := filepath.Join(root, "fixtures")
	if err := os.MkdirAll(filepath.Join(fixtures, "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cases, filepath.FromSlash(id))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, CaseFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, content := range extra {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return Load(cases, dir, fixtures)
}

func TestForEachExpansion(t *testing.T) {
	c, err := writeCase(t, "a/b", `{"id":"a/b","title":"t","for_each":{"var":"value","values":["x","y","z"]},
		"steps":[{"run":["--preset","{{value}}"],"expect":{"stdout":["{{index}}"]}}]}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	xs := c.Expand()
	if len(xs) != 3 {
		t.Fatalf("want 3 expansions, got %d", len(xs))
	}
	for i, want := range []string{"x", "y", "z"} {
		if xs[i].ID != "a/b["+string(rune('0'+i))+"]" {
			t.Errorf("expansion %d id = %q", i, xs[i].ID)
		}
		vars := Vars{VarProject: "/p", VarHome: "/h", VarBin: "/b"}.With(xs[i].Vars)
		a, err := xs[i].Action(0, vars)
		if err != nil {
			t.Fatal(err)
		}
		if a.Argv[1] != want {
			t.Errorf("expansion %d argv = %v, want %q", i, a.Argv, want)
		}
	}
	single, err := writeCase(t, "s", `{"id":"s","title":"t","steps":[{"run":[]}]}`, nil)
	if err != nil || len(single.Expand()) != 1 || single.Expand()[0].ID != "s" {
		t.Fatalf("a case without for_each is one expansion with its own id: %v", err)
	}
}

func TestLoadAcceptsCapturesAndFromFiles(t *testing.T) {
	_, err := writeCase(t, "ok", `{"id":"ok","title":"t",
		"project":{"files":{".jitso/briefs/b.md":{"from":"brief.md"},"x":{"content":"#!/bin/sh\n","mode":"0755"},"gone":null,"ln":{"symlink":"{{HOME}}/t"}}},
		"orca":{"cwd":null,"screens":["a"],"errors":{"terminal create":{"code":"c","message":"m"}}},
		"steps":[
		 {"run":["launch"],"capture":{"LAUNCH_ID":{"from":"stdout","regex":"JITSO (L\\S+)"}},
		  "expect":{"exit":0,"files":[{"path":".jitso/launches/{{LAUNCH_ID}}/run.sh","executable":true}]}},
		 {"exec":"run_sh","launch":"{{LAUNCH_ID}}","expect":{"claude":{"ran":true}}},
		 {"write":{"project":{"late.md":"text"}},"expect":{"registry":{"count":1}}},
		 {"hook":"guard","input":{"tool_name":"Bash"},"expect":{"decision":"none"}},
		 {"hook":"guard","input":{"tool_input":{"command":"{{JITSO}} launch"}},"expect":{"reason":["{{JITSO}}"]}},
		 {"run":["{{JITSO}}"],"expect":{"stdout":["{{JITSO}}"]}}]}`,
		map[string]string{"brief.md": "brief at {{PROJECT}}"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLoadErrors(t *testing.T) {
	cases := []struct{ name, id, body, want string }{
		{"unknown top key", "x", `{"id":"x","title":"t","stpes":[],"steps":[{"run":[]}]}`, `unknown key "stpes"`},
		{"id mismatch", "x", `{"id":"y","title":"t","steps":[{"run":[]}]}`, "must equal the folder path"},
		{"status key is gone", "x", `{"id":"x","title":"t","status":{"go":"pass"},"steps":[{"run":[]}]}`, `unknown key "status"`},
		{"unknown placeholder", "x", `{"id":"x","title":"t","steps":[{"run":["{{NOPE}}"]}]}`, "unknown placeholder {{NOPE}}"},
		{"capture used in setup", "x", `{"id":"x","title":"t","project":{"files":{"a":"{{L}}"}},"steps":[{"run":[],"capture":{"L":{"from":"stdout","regex":"(.)"}}}]}`, "unknown placeholder {{L}}"},
		{"bad regex", "x", `{"id":"x","title":"t","steps":[{"run":[],"expect":{"stdout":[{"regex":"(("}]}}]}`, "bad regex"},
		{"two step kinds", "x", `{"id":"x","title":"t","steps":[{"run":[],"hook":"guard"}]}`, "exactly one of run, hook, exec, write"},
		{"expect on wrong step", "x", `{"id":"x","title":"t","steps":[{"write":{},"expect":{"stdout":["a"]}}]}`, "not available on a \"write\" step"},
		{"key of another kind", "x", `{"id":"x","title":"t","steps":[{"run":[],"input":{}}]}`, `unknown key "input"`},
		{"path escapes", "x", `{"id":"x","title":"t","project":{"files":{"../out":"x"}},"steps":[{"run":[]}]}`, "stay inside"},
		{"missing from file", "x", `{"id":"x","title":"t","project":{"files":{"a":{"from":"nope.md"}}},"steps":[{"run":[]}]}`, "nope.md"},
		{"bad scenario key", "x", `{"id":"x","title":"t","orca":{"screen":["a"]},"steps":[{"run":[]}]}`, `unknown key "screen"`},
		{"capture shadows built-in", "x", `{"id":"x","title":"t","steps":[{"run":[],"capture":{"PROJECT":{"from":"stdout","regex":"(.)"}}}]}`, "shadows"},
		{"bad decision", "x", `{"id":"x","title":"t","steps":[{"hook":"guard","input":{},"expect":{"decision":"allowed"}}]}`, "want allow, deny, ask or none"},
		{"no steps", "x", `{"id":"x","title":"t","steps":[]}`, "at least one step"},
		{"capture shadows JITSO", "x", `{"id":"x","title":"t","steps":[{"run":[],"capture":{"JITSO":{"from":"stdout","regex":"(.)"}}}]}`, "shadows"},
		{"for_each JITSO var", "x", `{"id":"x","title":"t","for_each":{"var":"JITSO","values":["a"]},"steps":[{"run":[]}]}`, "not a free placeholder name"},
		{"for_each reserved var", "x", `{"id":"x","title":"t","for_each":{"var":"HOME","values":["a"]},"steps":[{"run":[]}]}`, "not a free placeholder name"},
	}
	for _, c := range cases {
		_, err := writeCase(t, c.id, c.body, nil)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want an error containing %q, got %v", c.name, c.want, err)
		}
	}
}
