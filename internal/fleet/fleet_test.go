package fleet

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/orca"
	"github.com/upex-galaxy/jitso/internal/registry"
)

const (
	idA = "L20261124T100000-a1a1"
	idB = "L20261124T100100-b2b2"
	idC = "L20261124T090000-c3c3"
)

func launch(id, parent, build, handle string) string {
	p := "null"
	if parent != "" {
		p = `"` + parent + `"`
	}
	return `{"v":2,"event":"launch","id":"` + id + `","parent":` + p + `,"root":"` + idA + `","depth":0,"ts":"2026-11-24T10:00:00.000Z","mode":"fleet","role":"operator","type":"worker","build":"` + build +
		`","harness":"claude","orca":{"run":null,"task":null,"terminal":"` + handle + `","title":"[jitso:` + id + `] ` + build + `"}}` + "\n"
}

func end(id string) string {
	return `{"v":2,"event":"end","id":"` + id + `","ts":"2026-11-24T10:30:00.000Z","source":"jitso_close","outcome_reported":"none","footer_path":null}` + "\n"
}

type fakeOrca struct {
	terminals []orca.TerminalInfo
	err       error
	calls     int
}

func (f *fakeOrca) ListTerminals() ([]orca.TerminalInfo, error) {
	f.calls++
	return f.terminals, f.err
}

func store(t *testing.T, text string) *registry.FileStore {
	t.Helper()
	dir := filepath.Join(t.TempDir(), ".jitso")
	s := registry.NewFileStore(dir)
	if text != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(s.Path(), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func TestParseArgs(t *testing.T) {
	ok := []struct {
		args []string
		want Options
	}{
		{nil, Options{}},
		{[]string{"--json"}, Options{JSON: true}},
		{[]string{"--all", "--json"}, Options{JSON: true, All: true}},
	}
	for _, tt := range ok {
		got, err := ParseArgs(tt.args)
		if err != nil || got != tt.want {
			t.Errorf("%q: got %+v, %v", tt.args, got, err)
		}
	}
	for _, args := range [][]string{{"x"}, {"--json", "--json"}, {"--all", "--all"}, {"--run", "r"}, {""}} {
		_, err := ParseArgs(args)
		var e *fail.Error
		if !errors.As(err, &e) || e.Kind != fail.KindRejected || e.Msg != Usage {
			t.Errorf("%q: got %v", args, err)
		}
	}
}

func TestRows(t *testing.T) {
	registryText := launch(idC, "", "closed-one", "term_c") + end(idC) +
		launch(idA, "", "conductor", "term_a") +
		launch(idB, idA, "worker", "term_b")
	terminals := []orca.TerminalInfo{
		{Handle: "term_a", Title: "[jitso:" + idA + "] conductor", Worktree: "/w", Branch: "main"},
		{Handle: "term_c", Title: "[jitso:" + idC + "] closed-one"},
		{Handle: "term_z", Title: "zsh", Worktree: "/other"},
		// A new handle that carries the mark of a known launch is explained.
		{Handle: "term_new", Title: "[jitso:" + idB + "] worker"},
	}
	launches, err := registry.ReadLaunches(store(t, registryText))
	if err != nil {
		t.Fatal(err)
	}
	type view struct{ ID, State, Handle string }
	views := func(rows []Row) []view {
		var v []view
		for _, r := range rows {
			v = append(v, view{r.ID, r.State, r.Handle})
		}
		return v
	}
	tests := []struct {
		name      string
		opts      Options
		orcaKnown bool
		want      []view
	}{
		{"default hides closed", Options{}, true, []view{{idA, Live, "term_a"}, {idB, Gone, "term_b"}, {"", Unlaunched, "term_z"}}},
		{"all shows closed first, in launch order", Options{All: true}, true, []view{{idC, Closed, "term_c"}, {idA, Live, "term_a"}, {idB, Gone, "term_b"}, {"", Unlaunched, "term_z"}}},
		{"without Orca nothing is live or unlaunched", Options{}, false, []view{{idA, Unknown, "term_a"}, {idB, Unknown, "term_b"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := terminals
			if !tt.orcaKnown {
				ts = nil
			}
			rows := Rows(tt.opts, launches, ts, tt.orcaKnown)
			if got := views(rows); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
	rows := Rows(Options{}, launches, terminals, true)
	if rows[0].Worktree != "/w" || rows[0].Branch != "main" || rows[0].Parent != "" || rows[1].Parent != idA {
		t.Errorf("live row %+v, child %+v", rows[0], rows[1])
	}
	if rows[0].Role != "operator" || rows[0].Harness != "claude" || rows[0].Mode != "fleet" {
		t.Errorf("registry fields not carried: %+v", rows[0])
	}
}

func TestRunDoesNotWrite(t *testing.T) {
	s := store(t, launch(idA, "", "conductor", "term_a"))
	before, _ := os.ReadFile(s.Path())
	f := &fakeOrca{terminals: []orca.TerminalInfo{{Handle: "term_a"}}}
	out, err := Run(Options{}, Deps{Store: s, Orca: f})
	if err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 || !strings.HasSuffix(out, "jitso fleet: 1 live") {
		t.Errorf("calls %d, out %q", f.calls, out)
	}
	if after, _ := os.ReadFile(s.Path()); string(after) != string(before) {
		t.Error("the registry changed")
	}
	if _, err := os.Stat(filepath.Join(s.Dir, registry.LockName)); !os.IsNotExist(err) {
		t.Errorf("a lock folder was left: %v", err)
	}
}

func TestRunErrors(t *testing.T) {
	boom := errors.New("orca terminal list failed")
	if _, err := Run(Options{}, Deps{Store: store(t, ""), Orca: &fakeOrca{err: boom}}); !errors.Is(err, boom) {
		t.Errorf("got %v", err)
	}
	out, err := Run(Options{}, Deps{Store: store(t, "")})
	if err != nil || out != "jitso fleet: no sessions" {
		t.Errorf("got %q, %v", out, err)
	}
}

func TestText(t *testing.T) {
	rows := []Row{
		{ID: idA, Role: "conductor", Type: "conductor", Harness: "claude", State: Live, Handle: "term_a", Title: "[jitso:" + idA + "] conductor"},
		{ID: idB, Parent: idA, Role: "operator", Type: "worker", Harness: "pi", State: Gone, Handle: "term_b", Title: "a\tb\nc"},
		{State: Unlaunched, Handle: "term_z", Title: "zsh"},
	}
	got := strings.Split(Text(rows), "\n")
	if len(got) != 5 {
		t.Fatalf("lines %q", got)
	}
	cols := func(line string) []string { return strings.Fields(line) }
	if !reflect.DeepEqual(cols(got[0]), []string{"ID", "ROLE/TYPE", "HARNESS", "STATE", "HANDLE", "PARENT", "TITLE"}) {
		t.Errorf("header %q", got[0])
	}
	if want := []string{idA, "conductor/conductor", "claude", "live", "term_a", "-", "[jitso:" + idA + "]", "conductor"}; !reflect.DeepEqual(cols(got[1]), want) {
		t.Errorf("row 1 %q", got[1])
	}
	// Control characters of a title never break a row.
	if want := []string{idB, "operator/worker", "pi", "gone", "term_b", idA, "a", "b", "c"}; !reflect.DeepEqual(cols(got[2]), want) {
		t.Errorf("row 2 %q", got[2])
	}
	if want := []string{"-", "-", "-", "unlaunched", "term_z", "-", "zsh"}; !reflect.DeepEqual(cols(got[3]), want) {
		t.Errorf("row 3 %q", got[3])
	}
	if got[4] != "jitso fleet: 1 live, 1 gone, 1 unlaunched" {
		t.Errorf("summary %q", got[4])
	}
	// The columns line up.
	if strings.Index(got[0], "STATE") != strings.Index(got[1], "live") {
		t.Errorf("not aligned:\n%s", strings.Join(got, "\n"))
	}
}

func TestSummary(t *testing.T) {
	tests := []struct {
		states []string
		want   string
	}{
		{nil, "jitso fleet: no sessions"},
		{[]string{Unlaunched, Live, Live, Closed, Gone, Unknown}, "jitso fleet: 2 live, 1 gone, 1 unknown, 1 closed, 1 unlaunched"},
	}
	for _, tt := range tests {
		var rows []Row
		for _, s := range tt.states {
			rows = append(rows, Row{State: s})
		}
		if got := Summary(rows); got != tt.want {
			t.Errorf("got %q, want %q", got, tt.want)
		}
	}
}

func TestJSON(t *testing.T) {
	got, err := JSON([]Row{{State: Unlaunched, Handle: "term_z", Title: "zsh"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"orca\": true,\n  \"sessions\": [\n    {\n" +
		"      \"id\": null,\n      \"parent\": null,\n      \"role\": null,\n      \"type\": null,\n      \"build\": null,\n      \"harness\": null,\n      \"mode\": null,\n" +
		"      \"state\": \"unlaunched\",\n      \"handle\": \"term_z\",\n      \"title\": \"zsh\",\n      \"worktree\": null,\n      \"branch\": null,\n      \"ts\": null\n    }\n  ]\n}"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	empty, _ := JSON(nil, false)
	if empty != "{\n  \"orca\": false,\n  \"sessions\": []\n}" {
		t.Errorf("empty %q", empty)
	}
}
