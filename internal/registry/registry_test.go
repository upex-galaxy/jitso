package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const ts = "2026-11-13T14:22:33.000Z"

func str(s string) *string { return &s }

func launch(id string) *LaunchEvent {
	return &LaunchEvent{ID: id, Root: id, TS: ts, Mode: "direct", Build: "haiku-explorer", BuildVersion: 1,
		Orca: OrcaRef{Terminal: "term_fake", Title: "[jitso:" + id + "] haiku-explorer"}}
}

// fakeClock advances only when Sleep is called.
type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time        { return c.t }
func (c *fakeClock) Sleep(d time.Duration) { c.t = c.t.Add(d) }

func newStore(t *testing.T) *FileStore {
	s := NewFileStore(filepath.Join(t.TempDir(), ".jitso"))
	return s
}

func readLines(t *testing.T, s *FileStore) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Error("the file must end with a newline")
	}
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func TestAppendLaunchShape(t *testing.T) {
	s := newStore(t)
	id := "L20261113T142233-ab12"
	if err := s.Append(launch(id)); err != nil {
		t.Fatal(err)
	}
	lines := readLines(t, s)
	if len(lines) != 1 {
		t.Fatalf("%d lines", len(lines))
	}
	got := lines[0]
	want := map[string]any{"v": 2.0, "event": "launch", "build": "haiku-explorer", "build_version": 1.0, "parent": nil, "cwd": nil, "transcript_path": nil,
		"mcp": []any{}, "mcp_excluded": []any{}, "skills_first": []any{}, "flags": []any{}, "route_scope": []any{},
		"orca": map[string]any{"run": nil, "task": nil, "terminal": "term_fake", "title": "[jitso:" + id + "] haiku-explorer"}}
	for k, v := range want {
		if !reflect.DeepEqual(got[k], v) {
			t.Errorf("%s = %#v, want %#v", k, got[k], v)
		}
	}
	if _, present := got["budget"]; present {
		t.Error("budget must be left out when unset")
	}
	for _, old := range []string{"preset", "preset_version"} {
		if _, present := got[old]; present {
			t.Errorf("a v2 line has no %q key", old)
		}
	}
	if _, err := os.Stat(filepath.Join(s.Dir, LockName)); !os.IsNotExist(err) {
		t.Error("the lock must be removed after the write")
	}
}

func TestAppendNoHTMLEscape(t *testing.T) {
	s := newStore(t)
	if err := s.Append(&EndEvent{ID: "L20261113T142233-ab12", TS: ts, Source: "reap", OutcomeReported: "none", FooterPath: str("<id> & <file>")}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(s.Path())
	if !strings.Contains(string(data), `"footer_path":"<id> & <file>"`) {
		t.Errorf("line %s", data)
	}
}

func TestValidateRejectsRetiredKinds(t *testing.T) {
	for _, kind := range []string{"analyzed", "rule_break"} {
		problems := Validate(map[string]any{"v": 2.0, "event": kind, "ts": ts})
		if len(problems) != 1 || problems[0] != "event must be one of launch|end|link" {
			t.Errorf("%s: %v", kind, problems)
		}
	}
}

func TestAppendRejectsInvalid(t *testing.T) {
	tests := []struct {
		name string
		rec  Record
		want string
	}{
		{"spanish mode", func() Record { l := launch("L20261113T142233-ab12"); l.Mode = "directo"; return l }(), "invalid launch event: mode must be one of direct|fleet"},
		{"bad ts", func() Record { l := launch("L20261113T142233-ab12"); l.TS = "not a date"; return l }(), "invalid launch event: ts must be an ISO timestamp"},
		{"empty build", func() Record { l := launch("L20261113T142233-ab12"); l.Build = ""; return l }(), "invalid launch event: build must be a non-empty string"},
		{"bad source", &EndEvent{ID: "x", TS: ts, Source: "nope", OutcomeReported: "none"}, "invalid end event: source must be one of worker_done|session_end_hook|jitso_close|reap"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStore(t)
			err := s.Append(tt.rec)
			if err == nil || !strings.HasPrefix(err.Error(), tt.want) {
				t.Fatalf("got %v, want %s", err, tt.want)
			}
			if _, err := os.Stat(s.Path()); !os.IsNotExist(err) {
				t.Error("nothing must be written")
			}
		})
	}
}

func TestLockBusyTimesOut(t *testing.T) {
	s := newStore(t)
	clock := &fakeClock{t: time.Now()}
	s.Now, s.Sleep = clock.Now, clock.Sleep
	if err := os.MkdirAll(filepath.Join(s.Dir, LockName), 0o755); err != nil {
		t.Fatal(err)
	}
	err := s.Append(launch("L20261113T142233-ab12"))
	want := "the registry lock " + filepath.Join(s.Dir, LockName) + " stayed busy for 5000 ms"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v, want %s", err, want)
	}
	if clock.t.Sub(time.Now()) < 4*time.Second {
		t.Error("the fake clock should have advanced about 5 s")
	}
}

func TestStaleLockIsTakenOver(t *testing.T) {
	s := newStore(t)
	lock := filepath.Join(s.Dir, LockName)
	if err := os.MkdirAll(lock, 0o755); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Minute)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(launch("L20261113T142233-ab12")); err != nil {
		t.Fatal(err)
	}
	if len(readLines(t, s)) != 1 {
		t.Error("want one line")
	}
}

func TestParseSkipsBadLines(t *testing.T) {
	good := `{"v":2,"event":"launch","id":"L20261113T142233-ab12","parent":null,"root":"L20261113T142233-ab12","depth":0,"ts":"` + ts + `","mode":"fleet","build":"p","transcript_path":5}`
	text := strings.Join([]string{
		"",
		"not json",
		`{"v":2,"event":"launch","id":"L1","root":"L1","depth":0,"ts":"` + ts + `","mode":"fleet","build":"p"}`, // no parent
		`{"v":2,"event":"launch","id":"L2","parent":null,"root":"L2","depth":0.5,"ts":"` + ts + `","mode":"fleet","build":"p"}`,
		`{"v":2,"event":"launch","id":"L3","parent":null,"root":"L3","depth":0,"ts":"` + ts + `","mode":"flota","build":"p"}`,
		`{"v":1,"event":"end","id":"x","ts":"` + ts + `","source":"reap","outcome_reported":"none"}`, // a v1 line
		`[1,2]`,
		"   ",
		good,
	}, "\n")
	events := Parse([]byte(text))
	if len(events) != 1 {
		t.Fatalf("got %d events: %v", len(events), events)
	}
	e := events[0]
	if e.Kind() != KindLaunch || e.ID() != "L20261113T142233-ab12" {
		t.Errorf("event %v", e)
	}
	if _, ok := e.String("transcript_path"); ok {
		t.Error("a non-string transcript_path is not a string")
	}
	if n, ok := e.Number("transcript_path"); !ok || n != 5 {
		t.Error("the line is kept with its field as written")
	}
}

func TestEventsMissingFile(t *testing.T) {
	events, err := newStore(t).Events()
	if err != nil || len(events) != 0 {
		t.Errorf("got %v, %v", events, err)
	}
}

func TestFold(t *testing.T) {
	s := newStore(t)
	a, b := "L20261113T142233-aaaa", "L20261113T142233-bbbb"
	first := launch(a)
	first.Build = "first"
	second := launch(a)
	second.Build = "second"
	recs := []Record{
		first,
		&EndEvent{ID: "L20261113T142233-cccc", TS: ts, Source: "reap", OutcomeReported: "none"},
		launch(b),
		second,
		&EndEvent{ID: b, TS: ts, Source: "jitso_close", OutcomeReported: "none", FooterPath: str("/f")},
	}
	for _, r := range recs {
		if err := s.Append(r); err != nil {
			t.Fatal(err)
		}
	}
	launches, err := ReadLaunches(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(launches) != 2 || launches[0].ID != a || launches[1].ID != b {
		t.Fatalf("launches %+v", launches)
	}
	la, _ := Find(launches, a)
	if p, _ := la.Launch.String("build"); p != "first" {
		t.Errorf("first launch line must win, got %s", p)
	}
	if la.Ended() {
		t.Error("a must not be ended")
	}
	lb, _ := Find(launches, b)
	if !lb.Ended() {
		t.Error("b must be ended")
	}
	if term, _ := lb.Launch.String("orca.terminal"); term != "term_fake" {
		t.Errorf("orca.terminal %q", term)
	}
}

func TestIsLaunchID(t *testing.T) {
	for id, want := range map[string]bool{
		"L20261113T142233-ab12": true, "L20261113T142233-AB12": false, "x": false, "L20261113T142233-ab123": false,
	} {
		if IsLaunchID(id) != want {
			t.Errorf("%s: want %v", id, want)
		}
	}
}

func TestValidateIDs(t *testing.T) {
	base := func(id any) map[string]any {
		return map[string]any{"v": 2.0, "event": "end", "id": id, "ts": ts, "source": "reap", "outcome_reported": "none"}
	}
	for id, want := range map[string]bool{
		"L1": true, "a.b-c_d": true, "-a": false, "": false, strings.Repeat("a", 64): true, strings.Repeat("a", 65): false,
		"a\u212a": false, "\u017f": false,
	} {
		if got := len(Validate(base(id))) == 0; got != want {
			t.Errorf("id %q: valid %v, want %v", id, got, want)
		}
	}
}

func TestParseTime(t *testing.T) {
	for s, want := range map[string]bool{
		ts: true, "2026-11-13T14:22:33Z": true, "2026-11-13T14:22:33.123456+03:00": true, "2026-11-13": true,
		"not a date": false, "": false, "2026-13-01": false,
	} {
		if _, ok := ParseTime(s); ok != want {
			t.Errorf("%q: %v, want %v", s, ok, want)
		}
	}
}

func TestValidateSchemaVersion(t *testing.T) {
	launch := func(extra string) string {
		return `{` + extra + `"event":"launch","id":"L1","parent":null,"root":"L1","depth":0,"ts":"` + ts + `","mode":"direct","build":"p"}`
	}
	for _, tt := range []struct {
		name string
		line string
		want []string
	}{
		{"v 2", launch(`"v":2,`), nil},
		{"v 2.0", launch(`"v":2.0,`), nil},
		{"v 1", launch(`"v":1,`), []string{"v must be 2"}},
		{"v 3", launch(`"v":3,`), []string{"v must be 2"}},
		{"v as a string", launch(`"v":"2",`), []string{"v must be 2"}},
		{"no v", launch(""), []string{"v must be 2"}},
		{"a v1 launch line with the old key", `{"v":1,"event":"launch","id":"L1","parent":null,"root":"L1","depth":0,"ts":"` + ts + `","mode":"direct","preset":"p"}`, []string{"v must be 2", "build must be a non-empty string"}},
		{"a v2 line with the old key only", `{"v":2,"event":"launch","id":"L1","parent":null,"root":"L1","depth":0,"ts":"` + ts + `","mode":"direct","preset":"p"}`, []string{"build must be a non-empty string"}},
		{"an end event at v 1", `{"v":1,"event":"end","id":"x","ts":"` + ts + `","source":"reap","outcome_reported":"none"}`, []string{"v must be 2"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var v any
			if err := json.Unmarshal([]byte(tt.line), &v); err != nil {
				t.Fatal(err)
			}
			if got := Validate(v); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Validate = %q, want %q", got, tt.want)
			}
			if got := len(Parse([]byte(tt.line))); (got == 1) != (tt.want == nil) {
				t.Errorf("Parse kept %d events", got)
			}
		})
	}
}

func TestCountV1(t *testing.T) {
	text := strings.Join([]string{
		`{"event":"launch","v":1,"id":"L1","preset":"p"}`,
		`{"v":1.0,"event":"end","id":"L1"}`,
		`{"v":2,"event":"end","id":"L1"}`,
		`{"v":"1"}`,
		`{"v":1`,
		`[1]`,
		"",
		"not json",
		`{"v":3}`,
	}, "\n")
	if got := CountV1([]byte(text)); got != 2 {
		t.Errorf("CountV1 = %d, want 2", got)
	}
	if got := CountV1(nil); got != 0 {
		t.Errorf("CountV1(nil) = %d", got)
	}
}
