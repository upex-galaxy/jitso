package reap

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/registry"
)

const (
	idA = "L20261113T142233-a3f9"
	idB = "L20261113T142300-b4a0"
)

func launch(id, orcaJSON, extra string) string {
	return `{"v":2,"event":"launch","id":"` + id + `","parent":null,"root":"` + id + `","depth":0,"ts":"2026-11-13T14:22:33.000Z","mode":"direct","build":"p","orca":` + orcaJSON + extra + "}\n"
}

func endOf(id string) string {
	return `{"v":2,"event":"end","id":"` + id + `","ts":"2026-11-13T14:30:00.000Z","source":"jitso_close","outcome_reported":"none","footer_path":null}` + "\n"
}

func orcaRef(terminal, run string) string {
	r := "null"
	if run != "" {
		r = `"` + run + `"`
	}
	return `{"run":` + r + `,"task":null,"terminal":"` + terminal + `","title":"t [jitso:` + idA + `]"}`
}

// fakeOrca answers terminal list and worker-list from scripts and records calls.
type fakeOrca struct {
	live    []string
	listErr error
	workers map[string]map[string]any // run -> worker-list result
	errs    map[string]error          // run -> worker-list error
	calls   []string
}

func (f *fakeOrca) ListHandles() ([]string, error) {
	f.calls = append(f.calls, "terminal list")
	return f.live, f.listErr
}

func (f *fakeOrca) Call(args []string) (map[string]any, error) {
	run := args[len(args)-1]
	f.calls = append(f.calls, strings.Join(args[:2], " ")+" "+run)
	if err := f.errs[run]; err != nil {
		return nil, err
	}
	return f.workers[run], nil
}

type harness struct {
	deps    Deps
	store   *registry.FileStore
	orca    *fakeOrca
	stderr  []string
	analyze []string
}

func newHarness(t *testing.T, registryText string, f *fakeOrca) *harness {
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
	h := &harness{store: store, orca: f}
	h.deps = Deps{
		Store: store,
		Orca:  f,
		Now:   func() time.Time { return time.Date(2026, 11, 13, 14, 23, 33, 0, time.UTC) },
		Analyze: func(id string) error {
			h.analyze = append(h.analyze, id)
			return nil
		},
		TranscriptExists: func(p string) bool { return p == "/there.jsonl" },
		Err:              func(line string) { h.stderr = append(h.stderr, line) },
	}
	return h
}

func (h *harness) events(t *testing.T) []registry.Event {
	t.Helper()
	events, err := h.store.Events()
	if err != nil {
		t.Fatal(err)
	}
	return events
}

func summary(ended, unlogged, analyzed int) string {
	return "jitso reap: " + strconv.Itoa(ended) + " launch(es) marked ended, " + strconv.Itoa(unlogged) + " unlogged worker(s), " + strconv.Itoa(analyzed) + " analyzed"
}

func TestParseArgs(t *testing.T) {
	long := strings.Repeat("a", 65)
	tests := []struct {
		name string
		args []string
		runs []string
		ok   bool
	}{
		{"none", nil, nil, true},
		{"one", []string{"--run", "run_1"}, []string{"run_1"}, true},
		{"two", []string{"--run", "a", "--run", "b"}, []string{"a", "b"}, true},
		{"same twice is kept for the caller to dedupe", []string{"--run", "a", "--run", "a"}, []string{"a", "a"}, true},
		{"64 characters", []string{"--run", long[:64]}, []string{long[:64]}, true},
		{"orca id punctuation", []string{"--run", "run:1.2-x_y"}, []string{"run:1.2-x_y"}, true},
		{"65 characters", []string{"--run", long}, nil, false},
		{"flag alone", []string{"--run"}, nil, false},
		{"unknown flag", []string{"--bogus"}, nil, false},
		{"bare word", []string{"extra"}, nil, false},
		{"equals form", []string{"--run=run_1"}, nil, false},
		{"short flag", []string{"-r"}, nil, false},
		{"bad after good", []string{"--run", "run_1", "--bogus"}, nil, false},
		{"empty value", []string{"--run", ""}, nil, false},
		{"leading dash", []string{"--run", "-x"}, nil, false},
		{"dot dot", []string{"--run", "a..b"}, nil, false},
		{"traversal", []string{"--run", "../x"}, nil, false},
		{"shell", []string{"--run", "run_1;ls"}, nil, false},
		{"space", []string{"--run", "a b; ls"}, nil, false},
		{"substitution", []string{"--run", "$(id)"}, nil, false},
		{"a trailing newline", []string{"--run", "run_1\n"}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runs, err := ParseArgs(tt.args)
			if tt.ok != (err == nil) {
				t.Fatalf("err %v", err)
			}
			if !tt.ok && (fail.Code(err) != 2 || fail.Text(err) != "JITSO REJECTED: "+Usage) {
				t.Errorf("rejection %q code %d", fail.Text(err), fail.Code(err))
			}
			if tt.ok && !reflect.DeepEqual(runs, tt.runs) && !(len(runs) == 0 && len(tt.runs) == 0) {
				t.Errorf("runs %v, want %v", runs, tt.runs)
			}
		})
	}
}

func TestEndPhase(t *testing.T) {
	tests := []struct {
		name     string
		registry string
		live     []string
		ended    int
		events   int
	}{
		{"empty registry", "", nil, 0, 0},
		{"gone terminal ends the launch", launch(idA, orcaRef("term_a", ""), ""), nil, 1, 2},
		{"listed terminal stays running", launch(idA, orcaRef("term_a", ""), ""), []string{"term_a"}, 0, 1},
		{"already ended is left alone", launch(idA, orcaRef("term_a", ""), "") + endOf(idA), nil, 0, 2},
		{"a null terminal handle is ended", launch(idA, `{"terminal":null,"title":"t"}`, ""), nil, 1, 2},
		{"an absent terminal handle is ended", launch(idA, `{"title":"t"}`, ""), nil, 1, 2},
		{"a number as the handle is ended", launch(idA, `{"terminal":7}`, ""), []string{"7"}, 1, 2},
		{"orca is null is ended", launch(idA, `null`, ""), nil, 1, 2},
		{"a launch with no handle is ended even when others are listed", launch(idA, `{"terminal":null}`, "") + launch(idB, orcaRef("term_b", ""), ""), []string{"term_b"}, 1, 3},
		{"only the gone one", launch(idA, orcaRef("term_a", ""), "") + launch(idB, orcaRef("term_b", ""), ""), []string{"term_b"}, 1, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.registry, &fakeOrca{live: tt.live})
			out, err := Run(nil, h.deps)
			if err != nil || out != summary(tt.ended, 0, 0) {
				t.Fatalf("out %q err %v", out, err)
			}
			events := h.events(t)
			if len(events) != tt.events {
				t.Fatalf("%d events, want %d", len(events), tt.events)
			}
			if tt.ended > 0 {
				end := events[len(events)-1]
				if src, _ := end.String("source"); end.Kind() != registry.KindEnd || src != "reap" {
					t.Errorf("last event %v", end)
				}
				if end["footer_path"] != nil {
					t.Errorf("footer_path %v, want null", end["footer_path"])
				}
				if d, _ := end.Number("duration_s"); d != 60 {
					t.Errorf("duration_s %v, want 60", d)
				}
			}
			if !reflect.DeepEqual(h.orca.calls, []string{"terminal list"}) {
				t.Errorf("orca calls %v", h.orca.calls)
			}
		})
	}
}

func TestSecondReapIsANoOp(t *testing.T) {
	h := newHarness(t, launch(idA, orcaRef("term_a", ""), ""), &fakeOrca{})
	for i, want := range []string{summary(1, 0, 0), summary(0, 0, 0)} {
		if out, err := Run(nil, h.deps); err != nil || out != want {
			t.Fatalf("run %d: %q %v, want %q", i+1, out, err, want)
		}
	}
	if n := len(h.events(t)); n != 2 {
		t.Errorf("%d events, want 2", n)
	}
}

func TestListFailureWritesNothing(t *testing.T) {
	h := newHarness(t, launch(idA, orcaRef("term_a", "run_1"), ""), &fakeOrca{listErr: errors.New("orca terminal list failed: boom")})
	out, err := Run(nil, h.deps)
	if out != "" || fail.Code(err) != 3 || fail.Text(err) != "jitso: orca terminal list failed: boom" {
		t.Errorf("out %q err %v", out, err)
	}
	if n := len(h.events(t)); n != 1 || len(h.orca.calls) != 1 {
		t.Errorf("%d events, calls %v", n, h.orca.calls)
	}
}

func workersResult(workers ...map[string]any) map[string]any {
	list := make([]any, len(workers))
	for i, w := range workers {
		list[i] = w
	}
	return map[string]any{"workers": list}
}

func TestAuditPhase(t *testing.T) {
	known := launch(idA, orcaRef("term_logged", "run_1"), "")
	mark := "x [jitso:" + idA + "]"
	tests := []struct {
		name     string
		workers  []map[string]any
		unlogged int
		evidence string // the evidence of the rule_break written, "" for none
	}{
		{"explained by handle", []map[string]any{{"agentTerminalHandle": "term_logged"}}, 0, ""},
		{"explained by nested handle", []map[string]any{{"resource": map[string]any{"terminalHandle": "term_logged"}}}, 0, ""},
		{"explained by a mark in a field", []map[string]any{{"agentTerminalHandle": "term_x", "title": mark}}, 0, ""},
		{"explained by a mark at depth", []map[string]any{{"agentTerminalHandle": "term_y", "meta": map[string]any{"tags": []any{"x", mark}}}}, 0, ""},
		{"a key holding the mark does not explain", []map[string]any{{"agentTerminalHandle": "term_k", "meta": map[string]any{mark: "v"}}}, 1, "worker term_k in run run_1 has no launch event"},
		{"unknown handle", []map[string]any{{"agentTerminalHandle": "term_rogue"}}, 1, "worker term_rogue in run run_1 has no launch event"},
		{"the mark of an unknown launch explains nothing", []map[string]any{{"agentTerminalHandle": "term_o", "title": "x [jitso:L20990101T000000-ffff]"}}, 1, "worker term_o in run run_1 has no launch event"},
		{"no handle", []map[string]any{{"title": "nameless"}}, 1, "worker (no terminal handle) in run run_1 has no launch event"},
		{"non-string handle falls back to nested", []map[string]any{{"agentTerminalHandle": 5.0, "resource": map[string]any{"terminalHandle": "term_logged"}}}, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeOrca{live: []string{"term_logged"}, workers: map[string]map[string]any{"run_1": workersResult(tt.workers...)}}
			h := newHarness(t, known, f)
			out, err := Run(nil, h.deps)
			if err != nil || out != summary(0, tt.unlogged, 0) {
				t.Fatalf("out %q err %v", out, err)
			}
			events := h.events(t)
			if tt.evidence == "" {
				if len(events) != 1 {
					t.Errorf("%d events, want 1", len(events))
				}
				return
			}
			rb := events[len(events)-1]
			if ev, _ := rb.String("evidence"); rb.Kind() != registry.KindRuleBreak || ev != tt.evidence || rb["id"] != nil {
				t.Errorf("rule_break %v", rb)
			}
			if kind, _ := rb.String("kind"); kind != "unlogged_worker" {
				t.Errorf("kind %q", kind)
			}
		})
	}
}

func TestRuleBreakIsWrittenOnce(t *testing.T) {
	f := &fakeOrca{live: []string{"term_logged", "term_rogue"}, workers: map[string]map[string]any{"run_1": workersResult(map[string]any{"agentTerminalHandle": "term_rogue"})}}
	h := newHarness(t, launch(idA, orcaRef("term_logged", "run_1"), ""), f)
	for i, want := range []int{1, 0} {
		if out, err := Run(nil, h.deps); err != nil || out != summary(0, want, 0) {
			t.Fatalf("run %d: %q %v (the count is what this reap wrote)", i+1, out, err)
		}
	}
	if n := len(h.events(t)); n != 2 {
		t.Errorf("%d events, want 2", n)
	}
}

func TestRunOrderAndDedup(t *testing.T) {
	rogue := workersResult(map[string]any{"agentTerminalHandle": "term_logged"}, map[string]any{"agentTerminalHandle": "term_rogue"})
	f := &fakeOrca{
		live:    []string{"term_logged", "term_rogue"},
		workers: map[string]map[string]any{"run_9": rogue, "run_8": rogue, "run_1": rogue},
	}
	h := newHarness(t, launch(idA, orcaRef("term_logged", "run_1"), ""), f)
	out, err := Run([]string{"run_9", "run_8", "run_9", "run_1"}, h.deps)
	if err != nil || out != summary(0, 1, 0) {
		t.Fatalf("out %q err %v", out, err)
	}
	want := []string{"terminal list", "orchestration worker-list run_9", "orchestration worker-list run_8", "orchestration worker-list run_1"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("calls %v, want %v", f.calls, want)
	}
	events := h.events(t)
	if len(events) != 2 {
		t.Fatalf("%d events: one rule_break for the worker across all runs, want 2", len(events))
	}
	if ev, _ := events[1].String("evidence"); ev != "worker term_rogue in run run_9 has no launch event" {
		t.Errorf("the evidence names the first run it was seen in: %q", ev)
	}
}

// TestUnloggedWorkerKeys pins TS-ODDITIES R4 (defect N-28): a worker with a
// handle is flagged once across runs and across reaps, a worker with no handle
// once per run, and a handle already named in the log is known.
func TestUnloggedWorkerKeys(t *testing.T) {
	rogue := workersResult(map[string]any{"agentTerminalHandle": "term_rogue"})
	nameless := workersResult(map[string]any{"title": "nameless"})
	old := `{"v":2,"event":"rule_break","id":null,"ts":"2026-11-13T14:30:00.000Z","kind":"unlogged_worker","evidence":"worker term_rogue in run run_old has no launch event"}` + "\n"
	tests := []struct {
		name     string
		registry string
		runs     []string
		workers  map[string]map[string]any
		written  int
	}{
		{"same handle in two runs", "", []string{"run_a", "run_b"}, map[string]map[string]any{"run_a": rogue, "run_b": rogue}, 1},
		{"a handle named by an older line in another run", old, []string{"run_a"}, map[string]map[string]any{"run_a": rogue}, 0},
		{"no handle in two runs is one line per run", "", []string{"run_a", "run_b"}, map[string]map[string]any{"run_a": nameless, "run_b": nameless}, 2},
		{"no handle again in a known run", `{"v":2,"event":"rule_break","id":null,"ts":"2026-11-13T14:30:00.000Z","kind":"unlogged_worker","evidence":"worker (no terminal handle) in run run_a has no launch event"}` + "\n", []string{"run_a", "run_b"}, map[string]map[string]any{"run_a": nameless, "run_b": nameless}, 1},
		{"another rule break kind is not a known worker", `{"v":2,"event":"rule_break","id":null,"ts":"2026-11-13T14:30:00.000Z","kind":"other","evidence":"worker term_rogue in run run_a has no launch event"}` + "\n", []string{"run_a"}, map[string]map[string]any{"run_a": rogue}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeOrca{workers: tt.workers}
			h := newHarness(t, tt.registry, f)
			before := len(h.events(t))
			out, err := Run(tt.runs, h.deps)
			if err != nil || out != summary(0, tt.written, 0) {
				t.Fatalf("out %q err %v", out, err)
			}
			if n := len(h.events(t)) - before; n != tt.written {
				t.Errorf("%d lines written, want %d", n, tt.written)
			}
		})
	}
}

func TestRegistryRunWithBadValueIsNotAudited(t *testing.T) {
	f := &fakeOrca{live: []string{"term_a"}}
	h := newHarness(t, launch(idA, orcaRef("term_a", "a b;ls"), ""), f)
	if _, err := Run(nil, h.deps); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.calls, []string{"terminal list"}) {
		t.Errorf("calls %v", f.calls)
	}
}

func TestRunUnknownToOrcaIsReportedAndSkipped(t *testing.T) {
	f := &fakeOrca{errs: map[string]error{"run_1": errors.New("orca orchestration worker-list failed: no such run")}}
	h := newHarness(t, launch(idA, orcaRef("term_a", "run_1"), ""), f)
	out, err := Run(nil, h.deps)
	if err != nil || out != summary(1, 0, 0) {
		t.Fatalf("out %q err %v", out, err)
	}
	want := []string{"jitso: could not list the workers of run run_1: orca orchestration worker-list failed: no such run"}
	if !reflect.DeepEqual(h.stderr, want) {
		t.Errorf("stderr %v, want %v", h.stderr, want)
	}
	if got, _ := h.events(t)[1].String("source"); got != "reap" {
		t.Error("the end phase still ran")
	}
}

func TestWorkersOf(t *testing.T) {
	obj := map[string]any{"a": 1.0}
	tests := []struct {
		name   string
		result map[string]any
		want   int
	}{
		{"workers key", map[string]any{"workers": []any{obj, obj}}, 2},
		{"workers key wins over an earlier list", map[string]any{"agents": []any{obj}, "workers": []any{obj, obj}}, 2},
		{"first list in key order", map[string]any{"b": []any{obj, obj}, "a": []any{obj}, "n": 3.0}, 1},
		{"non-objects are dropped", map[string]any{"workers": []any{obj, "x", nil, 4.0, []any{}}}, 1},
		{"no list", map[string]any{"workers": "none"}, 0},
		{"empty result", map[string]any{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := len(workersOf(tt.result)); got != tt.want {
				t.Errorf("got %d workers, want %d", got, tt.want)
			}
		})
	}
}

func TestAnalyzePhase(t *testing.T) {
	withTranscript := func(id string) string {
		return launch(id, orcaRef("term_a", ""), `,"transcript_path":"/there.jsonl"`)
	}
	analyzed := `{"v":2,"event":"analyzed","id":"` + idA + `","ts":"2026-11-13T15:00:00.000Z","analyzer":"x","peak_context":1,"cost_usd_list":0,"verdict":"unknown","tokens":{"input":0,"cache_read":0,"cache_write":0,"output":0}}` + "\n"
	tests := []struct {
		name     string
		registry string
		live     []string
		ended    int
		analyzed int
		calls    []string
	}{
		{"just ended with a transcript", withTranscript(idA), nil, 1, 1, []string{idA}},
		{"ended earlier with a transcript", withTranscript(idA) + endOf(idA), []string{"term_a"}, 0, 1, []string{idA}},
		{"running launch is not analyzed", withTranscript(idA), []string{"term_a"}, 0, 0, nil},
		{"already analyzed", withTranscript(idA) + endOf(idA) + analyzed, nil, 0, 0, nil},
		{"transcript not on disk", launch(idA, orcaRef("term_a", ""), `,"transcript_path":"/missing.jsonl"`), nil, 1, 0, nil},
		{"transcript path null", launch(idA, orcaRef("term_a", ""), `,"transcript_path":null`), nil, 1, 0, nil},
		{"no transcript path", launch(idA, orcaRef("term_a", ""), ""), nil, 1, 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.registry, &fakeOrca{live: tt.live})
			out, err := Run(nil, h.deps)
			if err != nil || out != summary(tt.ended, 0, tt.analyzed) {
				t.Fatalf("out %q err %v", out, err)
			}
			if !reflect.DeepEqual(h.analyze, tt.calls) && !(len(h.analyze) == 0 && len(tt.calls) == 0) {
				t.Errorf("analyze calls %v, want %v", h.analyze, tt.calls)
			}
			if len(h.stderr) != 0 {
				t.Errorf("stderr %v", h.stderr)
			}
		})
	}
}

func TestAnalyzeFailureIsReportedNotFatal(t *testing.T) {
	text := launch(idA, orcaRef("term_a", ""), `,"transcript_path":"/there.jsonl"`)
	tests := []struct {
		name   string
		err    error
		stderr []string
	}{
		{"plain error", errors.New("open prices.json: no such file"), []string{"jitso: could not analyze " + idA + ": open prices.json: no such file"}},
		{"rejection says why, then that reap moved on", fail.Rejectedf("launch %s has no transcript on disk yet", idA),
			[]string{"JITSO REJECTED: launch " + idA + " has no transcript on disk yet", "jitso: could not analyze " + idA}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, text, &fakeOrca{})
			h.deps.Analyze = func(string) error { return tt.err }
			out, err := Run(nil, h.deps)
			if err != nil || out != summary(1, 0, 0) {
				t.Fatalf("out %q err %v", out, err)
			}
			if !reflect.DeepEqual(h.stderr, tt.stderr) {
				t.Errorf("stderr %q, want %q", h.stderr, tt.stderr)
			}
		})
	}
}
