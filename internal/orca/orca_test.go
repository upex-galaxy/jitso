package orca

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// script is a Runner that answers from a table and records the calls.
type script struct {
	answers map[string]Result
	calls   [][]string
}

func (s *script) run(args []string) Result {
	s.calls = append(s.calls, args)
	if r, ok := s.answers[strings.Join(args[:2], " ")]; ok {
		return r
	}
	return Result{Status: 0, Stdout: `{"ok":true,"result":{"state":"ready"}}`}
}

func TestCall(t *testing.T) {
	tests := []struct {
		name     string
		res      Result
		wantErr  string
		wantCode string
	}{
		{"ok", Result{0, `{"ok":true,"result":{"a":1}}`, ""}, "", ""},
		{"no json", Result{1, "boom", "  orca: not running \n"}, "orca terminal list gave no JSON (exit 1): orca: not running", ""},
		{"no json no status", Result{-1, "", "exec: not found"}, "orca terminal list gave no JSON (exit null): exec: not found", ""},
		{"failed with message", Result{1, `{"ok":false,"error":{"code":"terminal_handle_stale","message":"gone"}}`, ""}, "orca terminal list failed: gone", CodeHandleStale},
		{"failed with code only", Result{1, `{"ok":false,"error":{"code":"x"}}`, ""}, "orca terminal list failed: x", "x"},
		{"failed bare", Result{1, `{"ok":false}`, ""}, "orca terminal list failed: unknown error", ""},
		{"failed numeric message", Result{1, `{"ok":false,"error":{"message":5}}`, ""}, "orca terminal list failed: 5", ""},
		{"not an object", Result{0, `null`, ""}, "orca terminal list failed: unknown error", ""},
		{"ok but exit code", Result{2, `{"ok":true}`, ""}, "orca terminal list exited 2", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &script{answers: map[string]Result{"terminal list": tt.res}}
			res, err := NewClient(s.run).Call([]string{"terminal", "list"})
			if tt.wantErr == "" {
				if err != nil || res == nil {
					t.Fatalf("got %v, %v", res, err)
				}
				return
			}
			var oe *Error
			if !errors.As(err, &oe) {
				t.Fatalf("want *Error, got %v", err)
			}
			if oe.Message != tt.wantErr || oe.Code != tt.wantCode {
				t.Errorf("got %q (code %q), want %q (code %q)", oe.Message, oe.Code, tt.wantErr, tt.wantCode)
			}
		})
	}
}

func TestTerminalVerbs(t *testing.T) {
	s := &script{answers: map[string]Result{
		"terminal create": {0, `{"ok":true,"result":{"terminal":{"handle":"term_1","worktreeId":"uuid::/repo/wt"}}}`, ""},
		"terminal read":   {0, `{"ok":true,"result":{"terminal":{"tail":["a",1,null]}}}`, ""},
		"terminal list":   {0, `{"ok":true,"result":{"terminals":[{"handle":"term_1"},{"x":1},{"handle":"term_2"}]}}`, ""},
		"terminal wait":   {1, `{"ok":false,"error":{"code":"timeout"}}`, ""},
	}}
	c := NewClient(s.run)
	term, err := c.CreateTerminal("", "[jitso:L1] p", "'/x/run.sh'")
	if err != nil || term.Handle != "term_1" || term.WorktreePath != "/repo/wt" {
		t.Fatalf("create: %+v %v", term, err)
	}
	if _, err := c.CreateTerminal("wt", "t", "c"); err != nil {
		t.Fatal(err)
	}
	screen, err := c.ReadScreen("term_1")
	if err != nil || screen != "a\n1\nnull" {
		t.Errorf("screen %q %v", screen, err)
	}
	if c.WaitIdle("term_1", 30000) {
		t.Error("a failed wait is false")
	}
	handles, err := c.ListHandles()
	if err != nil || !reflect.DeepEqual(handles, []string{"term_1", "term_2"}) {
		t.Errorf("handles %v %v", handles, err)
	}
	if err := c.SendText("term_1", "\x1b[B"); err != nil {
		t.Error(err)
	}
	if err := c.CloseTerminal("term_1"); err != nil {
		t.Error(err)
	}
	want := [][]string{
		{"terminal", "create", "--title", "[jitso:L1] p", "--command", "'/x/run.sh'"},
		{"terminal", "create", "--worktree", "wt", "--title", "t", "--command", "c"},
		{"terminal", "read", "--terminal", "term_1", "--screen"},
		{"terminal", "wait", "--terminal", "term_1", "--for", "tui-idle", "--timeout-ms", "30000"},
		{"terminal", "list"},
		{"terminal", "send", "--terminal", "term_1", "--text", "\x1b[B"},
		{"terminal", "close", "--terminal", "term_1"},
	}
	if !reflect.DeepEqual(s.calls, want) {
		t.Errorf("calls\n%q\nwant\n%q", s.calls, want)
	}
}

func TestCreateTerminalWithoutHandle(t *testing.T) {
	s := &script{answers: map[string]Result{"terminal create": {0, `{"ok":true,"result":{}}`, ""}}}
	_, err := NewClient(s.run).CreateTerminal("", "t", "c")
	if err == nil || err.Error() != "orca terminal create returned no handle" {
		t.Errorf("got %v", err)
	}
}

func TestWorktreePath(t *testing.T) {
	if p, ok := WorktreePath("u::/a::b"); !ok || p != "/a::b" {
		t.Errorf("got %q %v", p, ok)
	}
	if _, ok := WorktreePath("plain"); ok {
		t.Error("no :: means no path")
	}
}

func TestCommand(t *testing.T) {
	env := func(m map[string]string) func(string) (string, bool) {
		return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
	}
	if got := Command(env(nil)); got != "orca" {
		t.Errorf("default %q", got)
	}
	if got := Command(env(map[string]string{CommandEnv: ""})); got != "orca" {
		t.Errorf("empty %q", got)
	}
	if got := Command(env(map[string]string{CommandEnv: "/bin/fake-orca"})); got != "/bin/fake-orca" {
		t.Errorf("set %q", got)
	}
}

func TestExecRunner(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs /bin/sh")
	}
	dir := t.TempDir()
	prog := filepath.Join(dir, "orca")
	// Prints its argv and whether stdin is closed (read returns at once).
	body := "#!/bin/sh\nread -r line && echo \"stdin:$line\"\necho \"args:$*\"\necho err >&2\nexit 4\n"
	if err := os.WriteFile(prog, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	res := ExecRunner(prog)([]string{"terminal", "list"})
	if res.Status != 4 || res.Stdout != "args:terminal list --json\n" || res.Stderr != "err\n" {
		t.Errorf("result %+v", res)
	}
	missing := ExecRunner(filepath.Join(dir, "missing"))([]string{"x"})
	if missing.Status != -1 || missing.Stderr == "" {
		t.Errorf("missing program: %+v", missing)
	}
}
