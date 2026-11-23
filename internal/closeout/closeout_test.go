package closeout

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/orca"
	"github.com/upex-galaxy/jitso/internal/registry"
)

const id = "L20261113T142233-a3f9"

func launchLine(extra, orcaJSON, ts string) string {
	return `{"v":2,"event":"launch","id":"` + id + `","parent":null,"root":"` + id + `","depth":0,"ts":"` + ts + `","mode":"direct","build":"p","orca":` + orcaJSON + extra + "}\n"
}

const endLine = `{"v":2,"event":"end","id":"` + id + `","ts":"2026-11-13T14:30:00.000Z","source":"worker_done","outcome_reported":"none","footer_path":null}` + "\n"

// fakeOrca records the calls and fails the ones it is told to.
type fakeOrca struct {
	calls                      []string
	screen                     string
	readErr, closeErr, listErr error
	listed                     []string
}

func (f *fakeOrca) ReadScreen(h string) (string, error) {
	f.calls = append(f.calls, "read "+h)
	return f.screen, f.readErr
}

func (f *fakeOrca) CloseTerminal(h string) error {
	f.calls = append(f.calls, "close "+h)
	return f.closeErr
}

func (f *fakeOrca) ListHandles() ([]string, error) {
	f.calls = append(f.calls, "list")
	return f.listed, f.listErr
}

var stale = &orca.Error{Code: orca.CodeHandleStale, Message: "orca terminal read failed: stale"}

func setup(t *testing.T, registryText string, f *fakeOrca) (Deps, *registry.FileStore) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), ".jitso")
	store := registry.NewFileStore(dir)
	if registryText != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(store.Path(), []byte(registryText), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	now := func() time.Time { return time.Date(2026, 11, 13, 14, 23, 33, 600e6, time.UTC) }
	return Deps{StateDir: dir, Store: store, Orca: f, Now: now}, store
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		ok   bool
	}{
		{"good", []string{id}, true},
		{"none", nil, false},
		{"two", []string{id, id}, false},
		{"empty", []string{""}, false},
		{"traversal", []string{"../../etc"}, false},
		{"lower-case L", []string{"l20261113t142233-a3f9"}, false},
		{"upper-case hex", []string{"L20261113T142233-A3F9"}, false},
		{"trailing newline", []string{id + "\n"}, false},
		{"flag", []string{"--help"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseArgs(tt.args)
			if tt.ok != (err == nil) {
				t.Fatalf("err %v", err)
			}
			if !tt.ok && (fail.Code(err) != 2 || fail.Text(err) != "JITSO REJECTED: "+Usage) {
				t.Errorf("rejection %q code %d", fail.Text(err), fail.Code(err))
			}
			if tt.ok && got != id {
				t.Errorf("id %q", got)
			}
		})
	}
}

func TestRun(t *testing.T) {
	running := launchLine("", `{"terminal":"term_a"}`, "2026-11-13T14:22:33.000Z")
	tests := []struct {
		name      string
		registry  string
		orca      fakeOrca
		code      int
		stderr    string
		stdout    string
		calls     []string
		events    int // events in the registry afterwards
		footer    string
		footerOK  bool
		footerNil bool // the end event's footer_path is null
	}{
		{
			name: "normal close", registry: running, orca: fakeOrca{screen: "done\n  footer"},
			stdout: "jitso: closed " + id + " (terminal term_a)", calls: []string{"read term_a", "close term_a", "list"},
			events: 2, footer: "done\n  footer\n", footerOK: true,
		},
		{
			name: "stale read still ends the launch", registry: running, orca: fakeOrca{readErr: stale},
			stdout: "jitso: closed " + id + " (terminal term_a)", calls: []string{"read term_a", "close term_a", "list"},
			events: 2, footerNil: true,
		},
		{
			name: "stale read and close", registry: running, orca: fakeOrca{readErr: stale, closeErr: stale},
			stdout: "jitso: closed " + id + " (terminal term_a)", calls: []string{"read term_a", "close term_a", "list"},
			events: 2, footerNil: true,
		},
		{
			name: "another read error writes no end event", registry: running, orca: fakeOrca{readErr: &orca.Error{Message: "orca terminal read failed: boom"}},
			code: 3, stderr: "jitso: orca terminal read failed: boom", calls: []string{"read term_a"}, events: 1,
		},
		{
			name: "close failure leaves the launch running", registry: running, orca: fakeOrca{closeErr: &orca.Error{Message: "orca terminal close failed: boom"}},
			code: 3, stderr: "jitso: orca terminal close failed: boom", calls: []string{"read term_a", "close term_a"},
			events: 1,
		},
		{
			name: "list failure", registry: running, orca: fakeOrca{listErr: &orca.Error{Message: "orca terminal list failed: boom"}},
			code: 3, stderr: "jitso: orca terminal list failed: boom", calls: []string{"read term_a", "close term_a", "list"},
			events: 1,
		},
		{
			name: "terminal still listed", registry: running, orca: fakeOrca{listed: []string{"term_a"}},
			code: 3, stderr: "jitso: terminal term_a is still open after close", calls: []string{"read term_a", "close term_a", "list"},
			events: 1,
		},
		{
			name: "another terminal listed is fine", registry: running, orca: fakeOrca{listed: []string{"term_b"}},
			stdout: "jitso: closed " + id + " (terminal term_a)", calls: []string{"read term_a", "close term_a", "list"},
			events: 2, footerOK: true,
		},
		{
			name: "already ended closes again without a second end", registry: running + endLine, orca: fakeOrca{},
			stdout: "jitso: closed " + id + " (terminal term_a)", calls: []string{"close term_a", "list"}, events: 2,
		},
		{
			name: "unknown launch", registry: "", orca: fakeOrca{},
			code: 2, stderr: "JITSO REJECTED: unknown launch " + id, events: 0,
		},
		{
			name: "no terminal: null", registry: launchLine("", `{"terminal":null}`, "2026-11-13T14:22:33.000Z"), orca: fakeOrca{},
			code: 2, stderr: "JITSO REJECTED: launch " + id + " has no terminal handle", events: 1,
		},
		{
			name: "no terminal: number", registry: launchLine("", `{"terminal":42}`, "2026-11-13T14:22:33.000Z"), orca: fakeOrca{},
			code: 2, stderr: "has no terminal handle", events: 1,
		},
		{
			name: "no terminal: orca is null", registry: launchLine("", `null`, "2026-11-13T14:22:33.000Z"), orca: fakeOrca{},
			code: 2, stderr: "has no terminal handle", events: 1,
		},
		{
			name: "no terminal: orca is a list", registry: launchLine("", `[]`, "2026-11-13T14:22:33.000Z"), orca: fakeOrca{},
			code: 2, stderr: "has no terminal handle", events: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.orca
			d, store := setup(t, tt.registry, &f)
			out, err := Run(id, d)
			if fail.Code(err) != tt.code || !strings.Contains(fail.Text(err), tt.stderr) || out != tt.stdout {
				t.Errorf("code %d stderr %q stdout %q; want %d %q %q", fail.Code(err), fail.Text(err), out, tt.code, tt.stderr, tt.stdout)
			}
			if !reflect.DeepEqual(f.calls, tt.calls) && !(len(f.calls) == 0 && len(tt.calls) == 0) {
				t.Errorf("orca calls %v, want %v", f.calls, tt.calls)
			}
			events, _ := store.Events()
			if len(events) != tt.events {
				t.Fatalf("%d events, want %d", len(events), tt.events)
			}
			footer := filepath.Join(d.StateDir, "launches", id, "footer.txt")
			data, readErr := os.ReadFile(footer)
			switch {
			case tt.footerOK && (readErr != nil || (tt.footer != "" && string(data) != tt.footer)):
				t.Errorf("footer %q err %v, want %q", data, readErr, tt.footer)
			case !tt.footerOK && readErr == nil:
				t.Errorf("a footer file exists but must not: %q", data)
			}
			if tt.footerOK || tt.footerNil {
				end := events[len(events)-1]
				if end.Kind() != registry.KindEnd {
					t.Fatalf("last event %s", end.Kind())
				}
				if src, _ := end.String("source"); src != "jitso_close" {
					t.Errorf("source %q", src)
				}
				if oc, _ := end.String("outcome_reported"); oc != "none" {
					t.Errorf("outcome_reported %q", oc)
				}
				got, has := end.String("footer_path")
				if tt.footerNil && (has || end["footer_path"] != nil) {
					t.Errorf("footer_path %v, want null", end["footer_path"])
				}
				if tt.footerOK && got != footer {
					t.Errorf("footer_path %q, want %q", got, footer)
				}
			}
		})
	}
}

// TestRunWritesNothingUntilTheTerminalIsGone pins defect N-28:
// a failure at the close or the list leaves no footer.txt and no end event, and
// a second close, once Orca answers, ends the launch.
func TestRunWritesNothingUntilTheTerminalIsGone(t *testing.T) {
	f := &fakeOrca{screen: "footer", closeErr: errors.New("boom")}
	d, store := setup(t, launchLine("", `{"terminal":"term_a"}`, "2026-11-13T14:22:33.000Z"), f)
	if _, err := Run(id, d); err == nil {
		t.Fatal("expected an error")
	}
	launches, _ := registry.ReadLaunches(store)
	if len(launches) != 1 || launches[0].Ended() {
		t.Error("the end event must not be written before the terminal is confirmed gone")
	}
	if _, err := os.Stat(filepath.Join(d.StateDir, "launches", id, "footer.txt")); err == nil {
		t.Error("footer.txt must not be written before the terminal is confirmed gone")
	}
	f.closeErr = nil
	if _, err := Run(id, d); err != nil {
		t.Fatal(err)
	}
	launches, _ = registry.ReadLaunches(store)
	if !launches[0].Ended() {
		t.Error("the retry ends the launch")
	}
	if data, err := os.ReadFile(filepath.Join(d.StateDir, "launches", id, "footer.txt")); err != nil || string(data) != "footer\n" {
		t.Errorf("footer %q err %v", data, err)
	}
}

func TestSeconds(t *testing.T) {
	base := time.Date(2026, 11, 13, 14, 22, 33, 0, time.UTC)
	tests := []struct {
		name string
		ts   string
		now  time.Time
		want int64
	}{
		{"whole", "2026-11-13T14:22:33.000Z", base.Add(60 * time.Second), 60},
		{"rounds half up", "2026-11-13T14:22:33.000Z", base.Add(1500 * time.Millisecond), 2},
		{"rounds down", "2026-11-13T14:22:33.000Z", base.Add(1499 * time.Millisecond), 1},
		{"future launch is zero", "2999-01-01T00:00:00.000Z", base, 0},
		{"unparsable is zero", "not a time", base, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := seconds(registry.Event{"ts": tt.ts}, tt.now); got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRunTokens(t *testing.T) {
	use := func(id string, in, out int) string {
		return `{"message":{"id":"` + id + `","usage":{"input_tokens":` + string(rune('0'+in)) + `,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":` + string(rune('0'+out)) + `}}}` + "\n"
	}
	tests := []struct {
		name    string
		files   map[string]string
		dir     string
		harness string
		tokens  string
		line    string
		warning bool
	}{
		{
			name:   "transcript and subagent",
			files:  map[string]string{"s.jsonl": use("a", 1, 2) + use("a", 1, 3), "s/subagents/x.jsonl": use("b", 4, 5)},
			tokens: `{"input":5,"cache_write":0,"cache_read":0,"output":8,"messages":2,"subagent_files":1}`,
			line:   "tokens: input 5, cache write 0, cache read 0, output 8 (2 messages, 1 subagent transcripts)",
		},
		{name: "absent transcript is silent", tokens: "null"},
		{name: "a folder is unreadable", dir: "s.jsonl", tokens: "null", warning: true},
		{name: "a claude launch counts", files: map[string]string{"s.jsonl": use("a", 1, 2)}, harness: "claude",
			tokens: `{"input":1,"cache_write":0,"cache_read":0,"output":2,"messages":1,"subagent_files":0}`,
			line:   "tokens: input 1, cache write 0, cache read 0, output 2 (1 messages, 0 subagent transcripts)"},
		{name: "another harness counts nothing, silently", files: map[string]string{"s.jsonl": use("a", 1, 2)}, harness: "pi", tokens: "null"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			for name, text := range tt.files {
				p := filepath.Join(home, name)
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tt.dir != "" {
				if err := os.MkdirAll(filepath.Join(home, tt.dir), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			quoted, _ := json.Marshal(filepath.Join(home, "s.jsonl"))
			extra := `,"transcript_path":` + string(quoted)
			if tt.harness != "" {
				extra += `,"harness":"` + tt.harness + `"`
			}
			reg := launchLine(extra, `{"terminal":"term_a"}`, "2026-11-13T14:22:33.000Z")
			d, store := setup(t, reg, &fakeOrca{})
			var warned []string
			d.Warn = func(s string) { warned = append(warned, s) }
			out, err := Run(id, d)
			if err != nil {
				t.Fatal(err)
			}
			wantOut := "jitso: closed " + id + " (terminal term_a)"
			if tt.line != "" {
				wantOut += "\n" + tt.line
			}
			if out != wantOut {
				t.Errorf("stdout %q, want %q", out, wantOut)
			}
			if (len(warned) == 1) != tt.warning || len(warned) > 1 {
				t.Errorf("warnings %v", warned)
			}
			launches, _ := registry.ReadLaunches(store)
			var want any
			if err := json.Unmarshal([]byte(tt.tokens), &want); err != nil {
				t.Fatal(err)
			}
			if got := launches[0].End["tokens"]; !reflect.DeepEqual(got, want) {
				t.Errorf("tokens %v, want %v", got, want)
			}
		})
	}
}
