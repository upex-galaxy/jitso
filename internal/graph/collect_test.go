package graph

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/upex-galaxy/jitso/internal/orca"
	"github.com/upex-galaxy/jitso/internal/registry"
)

// fakeOrca answers from fields and records the calls.
type fakeOrca struct {
	terminals    []orca.TerminalInfo
	messages     []orca.Message
	coordinators map[string]string
	workers      map[string]map[string]string

	runErr, workerErr error
	listErr, inboxErr error
	calls             []string
}

func (f *fakeOrca) ListTerminals() ([]orca.TerminalInfo, error) {
	f.calls = append(f.calls, "list")
	return f.terminals, f.listErr
}

func (f *fakeOrca) Messages(limit int) ([]orca.Message, error) {
	f.calls = append(f.calls, "inbox")
	return f.messages, f.inboxErr
}

func (f *fakeOrca) RunCoordinators() (map[string]string, error) {
	f.calls = append(f.calls, "runs")
	return f.coordinators, f.runErr
}

func (f *fakeOrca) RunWorkers(run string) (map[string]string, error) {
	f.calls = append(f.calls, "workers "+run)
	if f.workerErr != nil {
		return map[string]string{}, f.workerErr
	}
	return f.workers[run], nil
}

func collector(t *testing.T, f *fakeOrca, now *time.Time) *Collector {
	return &Collector{
		Store: registry.NewFileStore(t.TempDir()),
		Orca:  func() (Orca, error) { return f, nil },
		Now:   func() time.Time { return *now },
	}
}

func terminals(handles ...string) []orca.TerminalInfo {
	var list []orca.TerminalInfo
	for _, h := range handles {
		list = append(list, orca.TerminalInfo{Handle: h, Title: h})
	}
	return list
}

func message(from, to, run string) orca.Message {
	return orca.Message{From: from, To: to, Type: "status", RunID: run, CreatedAt: "2026-11-25T10:00:00Z"}
}

func TestDispatchWorkersAreRemembered(t *testing.T) {
	now := time.Unix(0, 0)
	f := &fakeOrca{
		terminals: terminals("term_a", "term_b"),
		messages:  []orca.Message{message("dispatch:d1", "term_a", "r1"), message("dispatch:d2", "term_a", "r1")},
		workers:   map[string]map[string]string{"r1": {"d1": "term_b"}},
	}
	c := collector(t, f, &now)
	snapshot := func() Snapshot {
		f.calls = nil
		s, err := c.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	s := snapshot()
	if want := []string{"list", "inbox", "workers r1"}; !reflect.DeepEqual(f.calls, want) {
		t.Errorf("first calls %v, want %v", f.calls, want)
	}
	if len(s.Synapses) != 1 || s.Synapses[0].From != "term_b" || s.Synapses[0].Forward != 1 {
		t.Errorf("synapses %+v", s.Synapses)
	}
	// A moment later nothing is asked again, the unknown dispatch included.
	now = now.Add(CacheTTL)
	snapshot()
	if want := []string{"list", "inbox"}; !reflect.DeepEqual(f.calls, want) {
		t.Errorf("second calls %v, want %v", f.calls, want)
	}
	// Later the unknown dispatch asks again and the known one is kept when the
	// refresh fails.
	now = now.Add(workerRetry)
	f.workerErr = errors.New("busy")
	s = snapshot()
	if want := []string{"list", "inbox", "workers r1"}; !reflect.DeepEqual(f.calls, want) {
		t.Errorf("third calls %v, want %v", f.calls, want)
	}
	if len(s.Synapses) != 1 || !slices.Contains(s.Notices, NoticeUnresolved) || s.Failure == nil {
		t.Errorf("after a failed refresh: %+v", s)
	}
}

func TestRunListIsAskedOnlyForRunAddresses(t *testing.T) {
	now := time.Unix(0, 0)
	heartbeats := message("term_a", "run:r1", "r1")
	heartbeats.Type = heartbeat
	f := &fakeOrca{terminals: terminals("term_a", "term_b"), messages: []orca.Message{heartbeats, message("term_a", "term_b", "")}}
	s, err := collector(t, f, &now).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"list", "inbox"}; !reflect.DeepEqual(f.calls, want) {
		t.Errorf("calls %v, want %v", f.calls, want)
	}
	if len(s.Synapses) != 1 || s.Synapses[0].Forward != 1 || len(s.Notices) != 0 || s.Failure != nil {
		t.Errorf("snapshot %+v", s)
	}
}

func TestRunListFailureKeepsTheDirectSynapses(t *testing.T) {
	now := time.Unix(0, 0)
	f := &fakeOrca{
		terminals: terminals("term_a", "term_b"),
		messages:  []orca.Message{message("term_b", "run:r1", "r1"), message("term_a", "term_b", "")},
		runErr:    errors.New("run-list failed"),
	}
	s, err := collector(t, f, &now).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Synapses) != 1 || !reflect.DeepEqual(s.Notices, []string{NoticeUnresolved}) || s.Failure != f.runErr {
		t.Errorf("snapshot %+v", s)
	}
}

func TestFullInboxIsSaid(t *testing.T) {
	now := time.Unix(0, 0)
	f := &fakeOrca{terminals: terminals("term_a", "term_b")}
	for range InboxLimit {
		f.messages = append(f.messages, message("term_a", "term_b", ""))
	}
	s, err := collector(t, f, &now).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.Notices, []string{NoticeTruncated}) || s.Failure != nil || s.Synapses[0].Forward != InboxLimit {
		t.Errorf("notices %v, failure %v", s.Notices, s.Failure)
	}
}

// A hand-written log whose parents form a cycle still ends.
func TestParentCycle(t *testing.T) {
	now := time.Unix(0, 0)
	dir := t.TempDir()
	line := func(id, parent string) string {
		return `{"v":2,"event":"launch","id":"` + id + `","parent":"` + parent + `","root":"` + id + `","depth":0,"ts":"2026-11-25T10:00:00.000Z","mode":"direct","build":"b"}` + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, registry.FileName), []byte(line("a", "b")+line("b", "a")+line("c", "c")), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &Collector{Store: registry.NewFileStore(dir), Orca: func() (Orca, error) { return nil, errors.New("missing") }, Now: func() time.Time { return now }}
	s, err := c.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Nodes) != 3 || len(s.Hierarchy) != 2 || s.Orca {
		t.Errorf("snapshot %+v", s)
	}
	out, err := s.JSON()
	if err != nil || !strings.Contains(out, `"handle": null`) {
		t.Errorf("%v %s", err, out)
	}
}

// A late call is said as late, and no further call follows it in that snapshot.
func TestLateOrca(t *testing.T) {
	late := &orca.Error{Code: orca.CodeTimeout, Message: "late"}
	two := []orca.Message{message("dispatch:d1", "term_a", "r1"), message("dispatch:d2", "term_a", "r2")}
	tests := []struct {
		name    string
		f       *fakeOrca
		calls   []string
		notices []string
	}{
		{"terminal list", &fakeOrca{listErr: late}, []string{"list"},
			[]string{"Orca did not answer in time (terminal list): only the registry is shown, and Orca messages are not read."}},
		{"inbox", &fakeOrca{inboxErr: late}, []string{"list", "inbox"},
			[]string{"Orca did not answer in time (orchestration inbox): only the synapses recorded with jitso link are shown."}},
		{"run list", &fakeOrca{messages: append([]orca.Message{message("term_a", "run:r1", "r1")}, two...), runErr: late}, []string{"list", "inbox", "runs"},
			[]string{"Orca did not answer in time (orchestration run-list): some message synapses are missing."}},
		{"worker list", &fakeOrca{messages: two, workerErr: late}, []string{"list", "inbox", "workers r1"},
			[]string{"Orca did not answer in time (orchestration worker-list): some message synapses are missing."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Unix(0, 0)
			tt.f.terminals = terminals("term_a", "term_b")
			s, err := collector(t, tt.f, &now).Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(tt.f.calls, tt.calls) || !reflect.DeepEqual(s.Notices, tt.notices) || s.Failure != late {
				t.Errorf("calls %v, notices %q, failure %v", tt.f.calls, s.Notices, s.Failure)
			}
		})
	}
}

func TestNoProject(t *testing.T) {
	now := time.Unix(0, 0)
	inside := orca.TerminalInfo{Handle: "term_a", Worktree: "/w/repo"}
	f := &fakeOrca{terminals: []orca.TerminalInfo{inside}}
	c := &Collector{Orca: func() (Orca, error) { return f, nil }, Now: func() time.Time { return now }}
	s, err := c.Snapshot()
	if err != nil || s.ProjectEmpty != NoProjectCheckout || len(s.Nodes) != 1 || s.Nodes[0].Project || s.Nodes[0].Folder != "repo" {
		t.Errorf("outside a checkout: %v %+v", err, s)
	}
	// Without a registry the checkouts are not asked for.
	c = collector(t, f, &now)
	registered := false
	c.Registered = func() bool { return registered }
	c.Checkouts = func() []string { return []string{"/w/repo"} }
	if s, _ = c.Snapshot(); s.ProjectEmpty != NoProjectRegistry || s.Nodes[0].Project {
		t.Errorf("no registry: %+v", s)
	}
	registered = true
	if s, _ = c.Snapshot(); s.ProjectEmpty != "" || !s.Nodes[0].Project {
		t.Errorf("with a registry: %+v", s)
	}
	c.Checkouts = nil
	if s, _ = c.Snapshot(); s.ProjectEmpty != NoProjectSession {
		t.Errorf("no session of the project: %+v", s)
	}
}

func TestFolderName(t *testing.T) {
	for path, want := range map[string]string{
		"": "", "/": "", "/a/b": "b", "/a/b/": "b", "b": "b", `C:\work\gamma`: "gamma", `C:\work\gamma\`: "gamma", `C:/work\mix`: "mix",
	} {
		if got := folderName(path); got != want {
			t.Errorf("%q: %q, want %q", path, got, want)
		}
	}
}

func TestInsideAny(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	sub := filepath.Join(repo, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	inside := insideAny([]string{repo, filepath.Join(root, "gone")})
	for folder, want := range map[string]bool{
		repo:                            true,
		repo + string(os.PathSeparator): true,
		filepath.Join(root, "gone"):     true,
		sub:                             false,
		root:                            false,
		filepath.Join(root, "other"):    false,
	} {
		if got := inside(folder); got != want {
			t.Errorf("%s: %v, want %v", folder, got, want)
		}
	}
}

func TestParseArgs(t *testing.T) {
	good := []struct {
		args []string
		want Options
	}{
		{nil, Options{}},
		{[]string{"--json"}, Options{JSON: true}},
		{[]string{"--port", "0"}, Options{}},
		{[]string{"--port", "65535"}, Options{Port: 65535}},
	}
	for _, tt := range good {
		if got, err := ParseArgs(tt.args); err != nil || got != tt.want {
			t.Errorf("%v: %+v %v", tt.args, got, err)
		}
	}
	for _, args := range [][]string{{"--port"}, {"--port", "65536"}, {"--port", "1", "--port", "2"}, {"--port=1"}, {"--json", "--port", "1"}, {"x"}} {
		if _, err := ParseArgs(args); err == nil {
			t.Errorf("%v was accepted", args)
		}
	}
}
