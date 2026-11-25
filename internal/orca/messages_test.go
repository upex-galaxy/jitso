package orca

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestMessagesKeepNoContent(t *testing.T) {
	body := `{"ok":true,"result":{"messages":[{"id":"m1","from_handle":"term_a","to_handle":"run:r1","type":"status",` +
		`"run_id":"r1","created_at":"2026-11-25T10:00:00Z","subject":"SECRET","body":"SECRET","payload":"SECRET"},` +
		`{"from_handle":"term_b","to_handle":"term_a","run_id":null}]}}`
	s := &script{answers: map[string]Result{"orchestration inbox": {0, body, ""}}}
	got, err := NewClient(s.run).Messages(7)
	if err != nil {
		t.Fatal(err)
	}
	want := []Message{
		{From: "term_a", To: "run:r1", Type: "status", RunID: "r1", CreatedAt: "2026-11-25T10:00:00Z"},
		{From: "term_b", To: "term_a"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
	if strings.Contains(fmt.Sprintf("%+v", got), "SECRET") {
		t.Error("content kept")
	}
	if call := []string{"orchestration", "inbox", "--limit", "7", "--full"}; !reflect.DeepEqual(s.calls, [][]string{call}) {
		t.Errorf("calls %q", s.calls)
	}
}

func TestMessagesErrorsCarryNoOutput(t *testing.T) {
	tests := []struct {
		name string
		res  Result
		want string
	}{
		{"not JSON", Result{1, "SECRET", "SECRET"}, "orca orchestration inbox gave no readable JSON (exit 1)"},
		{"failed with a code", Result{1, `{"ok":false,"error":{"code":"x","message":"SECRET"}}`, "SECRET"}, "orca orchestration inbox failed (x)"},
		{"failed bare", Result{1, `{"ok":false}`, "SECRET"}, "orca orchestration inbox failed (exit 1)"},
		{"ok but exit code", Result{2, `{"ok":true}`, "SECRET"}, "orca orchestration inbox exited 2"},
	}
	for _, tt := range tests {
		s := &script{answers: map[string]Result{"orchestration inbox": tt.res}}
		_, err := NewClient(s.run).Messages(1)
		if err == nil || err.Error() != tt.want {
			t.Errorf("%s: %v, want %q", tt.name, err, tt.want)
		}
	}
}

func TestRunCoordinatorsAndWorkersFollowTheCursor(t *testing.T) {
	var calls [][]string
	c := NewClient(func(args []string) Result {
		calls = append(calls, args)
		last := args[len(args)-1]
		switch {
		case args[1] == "run-list" && last == "--full":
			return Result{0, `{"ok":true,"result":{"runs":[{"id":"r1","coordinator_handle":"term_a"},{"id":"r2","coordinator_handle":null}],"nextCursor":"c1"}}`, ""}
		case args[1] == "run-list":
			return Result{0, `{"ok":true,"result":{"runs":[{"id":"r3","coordinator_handle":"term_c"}],"nextCursor":null}}`, ""}
		case args[1] == "worker-list" && last == "--full":
			return Result{0, `{"ok":true,"result":{"workers":[{"dispatchId":"d1","agentTerminalHandle":"term_a"},{"dispatchId":"d2","agentTerminalHandle":null,"resource":{"terminalHandle":"term_b"}}],"page":{"hasMore":true,"nextCursor":"c2"}}}`, ""}
		case args[1] == "worker-list":
			return Result{0, `{"ok":true,"result":{"workers":[{"dispatchId":"d3","agentTerminalHandle":null}],"page":{"hasMore":false,"nextCursor":null}}}`, ""}
		}
		return Result{0, `{"ok":true,"result":{}}`, ""}
	})
	runs, err := c.RunCoordinators()
	if err != nil || !reflect.DeepEqual(runs, map[string]string{"r1": "term_a", "r3": "term_c"}) {
		t.Errorf("runs %v %v", runs, err)
	}
	workers, err := c.RunWorkers("r1")
	if err != nil || !reflect.DeepEqual(workers, map[string]string{"d1": "term_a", "d2": "term_b"}) {
		t.Errorf("workers %v %v", workers, err)
	}
	if err := c.SwitchTerminal("term_a"); err != nil {
		t.Error(err)
	}
	want := [][]string{
		{"orchestration", "run-list", "--full"},
		{"orchestration", "run-list", "--full", "--cursor", "c1"},
		{"orchestration", "worker-list", "--run", "r1", "--limit", "100", "--full"},
		{"orchestration", "worker-list", "--run", "r1", "--limit", "100", "--full", "--cursor", "c2"},
		{"terminal", "switch", "--terminal", "term_a"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("calls\n%q\nwant\n%q", calls, want)
	}
}

func TestACursorThatNeverEndsStops(t *testing.T) {
	calls := 0
	c := NewClient(func([]string) Result {
		calls++
		return Result{0, `{"ok":true,"result":{"runs":[],"nextCursor":"again"}}`, ""}
	})
	if _, err := c.RunCoordinators(); err != nil || calls != maxPages {
		t.Errorf("%d calls, %v", calls, err)
	}
}
