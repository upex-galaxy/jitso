package analyze

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/registry"
)

const pricesJSON = `{"verified_on":"2026-11-13","models":{
 "sonnet":{"input":2.0,"cache_read":0.1,"cache_write_5m":2.5,"cache_write_1h":4.0,"output":10.0},
 "opus":{"input":4.0,"cache_read":0.2,"cache_write_5m":5.0,"cache_write_1h":8.0,"output":20.0},
 "haiku":{"input":0.1,"cache_read":0.01,"cache_write_5m":0.125,"cache_write_1h":0.2,"output":0.5}}}`

// mainTranscript has 3 requests (m1 split over two lines, m2, m3), 2 secret
// reads (Bash and Read), a line that is not JSON and a user line.
const mainTranscript = `{"type":"user","timestamp":"2026-11-13T05:00:00.000Z","message":{"role":"user","content":[{"type":"tool_result","content":"X"}]}}
{"type":"assistant","uuid":"a","timestamp":"2026-11-13T05:01:00.000Z","message":{"id":"m1","model":"claude-sonnet-5-5","usage":{"input_tokens":10000,"cache_read_input_tokens":1000000,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":200000},"output_tokens":50000},"content":[{"type":"tool_use","name":"Bash","input":{"command":"cat .env.fake"}}]}}
{"type":"assistant","uuid":"b","timestamp":"2026-11-13T05:01:01.000Z","message":{"id":"m1","model":"claude-sonnet-5-5","usage":{"input_tokens":99,"cache_read_input_tokens":99,"output_tokens":99},"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/repo/.auth/s.json"}}]}}
{"type":"assistant","uuid":"c","timestamp":"2026-11-13T05:30:00.000Z","message":{"id":"m2","model":"claude-sonnet-5-5","usage":{"input_tokens":5000,"cache_read_input_tokens":3000000,"cache_creation_input_tokens":100000,"output_tokens":20000},"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/repo/README.md"}},{"type":"tool_use","name":"Bash","input":{"command":"echo hi"}},{"type":"tool_use","name":"Read","input":{"file_path":"/repo/.env.example"}}]}}
{ this line is not json
{"type":"assistant","uuid":"d","timestamp":"2026-11-13T06:30:00.000Z","message":{"id":"m3","model":"claude-haiku-5-5","usage":{"input_tokens":100000,"cache_creation":{"ephemeral_5m_input_tokens":40000,"ephemeral_1h_input_tokens":0},"output_tokens":10000},"content":[{"type":"text","text":"x"}]}}
`

const subTranscript = `{"type":"assistant","uuid":"s","timestamp":"2026-11-13T05:10:00.000Z","message":{"id":"s1","model":"claude-sonnet-5-5","usage":{"input_tokens":1000,"cache_read_input_tokens":500000,"cache_creation":{"ephemeral_5m_input_tokens":50000,"ephemeral_1h_input_tokens":0},"output_tokens":5000},"content":[{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}
`

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustPrices(t *testing.T) Prices {
	t.Helper()
	p, err := ParsePrices([]byte(pricesJSON))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestIsSecretPath(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"cat .env", true},
		{"cat .env.local", true},
		{"/repo/.env.fake-contract", true},
		{"/repo/.env.example", false},
		{"/repo/.env.staging.schema", false},
		{".env.example.bak", true},
		{"cat .env.example && echo", false},
		{"cat .env;ls", true},
		{"(cat .env)", true},
		{"cat .envrc", false},
		{"cat .env/x", false},
		{"cat foo.env", false},
		{"cat 'a'.env", true},
		{"x=.env", true},
		{"/repo/.auth/session.json", true},
		{".auth", true},
		{"cat .auth", true},
		{"cat .author", false},
		{"cat x.auth/y", false},
		{"/repo/.claude/settings.local.json", true},
		{"settings.local.json.bak", true},
		{"git check-ignore -v .jitso/x", false},
		{"cat .env", true},
		{"cat\u0085.env", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			if got := IsSecretPath(tt.text); got != tt.want {
				t.Errorf("IsSecretPath(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestTouchesSecret(t *testing.T) {
	tests := []struct {
		name  string
		tool  string
		input any
		want  bool
	}{
		{"read", "Read", map[string]any{"file_path": "/r/.env"}, true},
		{"grep path", "Grep", map[string]any{"pattern": "x", "path": "/r/.env"}, true},
		{"write is not counted", "Write", map[string]any{"file_path": "/r/.env"}, false},
		{"bash", "Bash", map[string]any{"command": "cat .env"}, true},
		{"bash other key", "Bash", map[string]any{"file_path": "/r/.env"}, false},
		{"non-string input", "Read", map[string]any{"file_path": 5.0}, false},
		{"not an object", "Read", "x", false},
		{"nil", "Read", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := touchesSecret(tt.tool, tt.input); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJSRound(t *testing.T) {
	tests := []struct{ in, want float64 }{{0.5, 1}, {1.5, 2}, {2.5, 3}, {-0.5, 0}, {-1.5, -1}, {-2.5, -2}, {2.4, 2}, {0, 0}}
	for _, tt := range tests {
		if got := jsRound(tt.in); got != tt.want {
			t.Errorf("jsRound(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestFamily(t *testing.T) {
	tests := []struct {
		model any
		want  string
	}{
		{"claude-opus-5", "opus"}, {"CLAUDE-HAIKU-5-5", "haiku"}, {"claude-sonnet-5-5", "sonnet"},
		{"", "sonnet"}, {nil, "sonnet"}, {42.0, "sonnet"}, {"opus-haiku", "opus"},
	}
	for _, tt := range tests {
		if got := family(tt.model); got != tt.want {
			t.Errorf("family(%v) = %q, want %q", tt.model, got, tt.want)
		}
	}
}

func TestDedupKey(t *testing.T) {
	tests := []struct {
		name         string
		id, uuid     any
		idOK, uuidOK bool
		key          string
		comparable   bool
	}{
		{"string id", "m1", "u", true, true, "s:m1", true},
		{"null id falls to uuid", nil, "u", true, true, "s:u", true},
		{"absent both", nil, nil, false, false, "u:", true},
		{"null uuid", nil, nil, false, true, "z:", true},
		{"number", 7.0, nil, true, false, "n:7", true},
		{"negative zero", negZero(), nil, true, false, "n:0", true},
		{"bool", true, nil, true, false, "b:true", true},
		{"object is never seen", map[string]any{}, nil, true, false, "", false},
		{"list is never seen", []any{}, nil, true, false, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, ok := dedupKey(tt.id, tt.uuid, tt.idOK, tt.uuidOK)
			if key != tt.key || ok != tt.comparable {
				t.Errorf("got (%q, %v), want (%q, %v)", key, ok, tt.key, tt.comparable)
			}
		})
	}
}

func negZero() float64 { z := 0.0; return -z * 1 }

func TestSessionContractTranscript(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "11111111-2222-3333-4444-555555555555.jsonl")
	write(t, main, mainTranscript)
	write(t, filepath.Join(dir, "11111111-2222-3333-4444-555555555555", "subagents", "agent-a.jsonl"), subTranscript)
	write(t, filepath.Join(dir, "11111111-2222-3333-4444-555555555555", "subagents", "notes.txt"), "ignored")

	a, err := Session(main, mustPrices(t))
	if err != nil {
		t.Fatal(err)
	}
	if a.Session != "11111111-2222-3333-4444-555555555555" || a.Files != 2 || a.SubagentFiles != 1 {
		t.Errorf("session/files: %+v", a)
	}
	if a.MainRequests != 3 || a.Requests != 4 {
		t.Errorf("requests: main %d total %d, want 3 and 4", a.MainRequests, a.Requests)
	}
	if a.MaxMainContextTokens != 3105000 {
		t.Errorf("peak %v, want 3105000", a.MaxMainContextTokens)
	}
	if want := (Tokens{Input: 116000, CacheRead: 4500000, CacheWrite: 390000, Output: 85000}); a.Tokens != want {
		t.Errorf("tokens %+v, want %+v", a.Tokens, want)
	}
	if a.CostUSDList != 2.427 {
		t.Errorf("cost %v, want 2.427", a.CostUSDList)
	}
	if a.SecretPathReads != 2 {
		t.Errorf("secret reads %d, want 2", a.SecretPathReads)
	}
	if a.DurationMin == nil || *a.DurationMin != 90 {
		t.Errorf("duration %v, want 90", a.DurationMin)
	}
	if a.First == nil || *a.First != "2026-11-13T05:00:00.000Z" || *a.Last != "2026-11-13T06:30:00.000Z" {
		t.Errorf("first/last %v %v", a.First, a.Last)
	}
	text, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"tool_use_counts":{"Bash":3,"Read":3}`,
		`"by_model":{"main:haiku":{"requests":1,"input":100000,"cache_read":0,"cache_write_5m":40000,"cache_write_1h":0,"output":10000,"usd":0.02},"main:sonnet":{"requests":2,`,
		`"sub:sonnet":{"requests":1,`,
	} {
		if !strings.Contains(string(text), want) {
			t.Errorf("json %s\nlacks %s", text, want)
		}
	}
}

func TestSessionMissingFile(t *testing.T) {
	_, err := Session(filepath.Join(t.TempDir(), "nope.jsonl"), mustPrices(t))
	if err == nil || !strings.Contains(err.Error(), "nope.jsonl does not exist") {
		t.Errorf("err %v", err)
	}
}

func TestSessionOneRequest(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "s.jsonl")
	write(t, main, `{"type":"assistant","uuid":"u","timestamp":"2026-11-13T05:00:00.000Z","message":{"id":"m1","model":"claude-haiku-5-5","usage":{"input_tokens":10,"output_tokens":5}}}`)
	a, err := Session(main, mustPrices(t))
	if err != nil {
		t.Fatal(err)
	}
	if a.DurationMin != nil || a.Files != 1 || a.CostUSDList != 0 {
		t.Errorf("%+v", a)
	}
	text, _ := json.Marshal(a)
	for _, want := range []string{`"duration_min":null`, `"tool_use_counts":{}`, `"cost_usd_list":0`} {
		if !strings.Contains(string(text), want) {
			t.Errorf("json %s lacks %s", text, want)
		}
	}
}

func TestScan(t *testing.T) {
	long := strings.Repeat("x", 200*1024)
	tests := []struct {
		name        string
		text        string
		rows        int
		input       float64
		tools       map[string]int
		stampCount  int
		secretReads int
	}{
		{"message split counts once, tools on every line",
			`{"type":"assistant","uuid":"a","timestamp":"t1","message":{"id":"m1","model":"claude-haiku-5-5","usage":{"input_tokens":100,"output_tokens":10},"content":[{"type":"tool_use","name":"Read","input":{"file_path":"a"}}]}}` + "\n" +
				`{"type":"assistant","uuid":"b","timestamp":"t2","message":{"id":"m1","model":"claude-haiku-5-5","usage":{"input_tokens":100,"output_tokens":10},"content":[{"type":"tool_use","name":"Read","input":{"file_path":"b"}}]}}`,
			1, 100, map[string]int{"Read": 2}, 2, 0},
		{"junk lines, user lines, empty and missing usage are ignored",
			"{ not json\n[1,2]\nnull\n\n" +
				`{"type":"user","timestamp":"t","message":{"usage":{"input_tokens":999}}}` + "\n" +
				`{"type":"assistant","message":{"id":"e1","usage":{}}}` + "\n" +
				`{"type":"assistant","message":{"id":"e2"}}` + "\n" +
				`{"type":"assistant","message":{"id":"ok","usage":{"input_tokens":7,"output_tokens":3}}}` + "\n",
			1, 7, map[string]int{}, 1, 0},
		{"a line over the scanner limit is read", `{"type":"assistant","message":{"id":"big","usage":{"input_tokens":1},"content":[{"type":"text","text":"` + long + `"}]}}` + "\n", 1, 1, map[string]int{}, 0, 0},
		{"no trailing newline", `{"type":"assistant","message":{"id":"x","usage":{"input_tokens":4}}}`, 1, 4, map[string]int{}, 0, 0},
		{"same id empty usage then real usage", `{"type":"assistant","message":{"id":"x","usage":{}}}` + "\n" + `{"type":"assistant","message":{"id":"x","usage":{"input_tokens":4}}}`, 1, 4, map[string]int{}, 0, 0},
		{"object id is never deduplicated", `{"type":"assistant","message":{"id":{"a":1},"usage":{"input_tokens":4}}}` + "\n" + `{"type":"assistant","message":{"id":{"a":1},"usage":{"input_tokens":4}}}`, 2, 8, map[string]int{}, 0, 0},
		{"no id and no uuid share one key", `{"type":"assistant","message":{"usage":{"input_tokens":4}}}` + "\n" + `{"type":"assistant","message":{"usage":{"input_tokens":4}}}`, 1, 4, map[string]int{}, 0, 0},
		{"secret read counted per call", `{"type":"assistant","message":{"id":"a","content":[{"type":"tool_use","name":"Read","input":{"file_path":"/r/.env"}},{"type":"tool_use","name":"Read","input":{"file_path":"/r/.env.example"}},{"type":"tool_use","name":"Write","input":{"file_path":"/r/.env"}}]}}`, 0, 0, map[string]int{"Read": 2, "Write": 1}, 0, 1},
		{"tool block without a string name is skipped", `{"type":"assistant","message":{"id":"a","content":["x",{"type":"tool_use","name":5},{"type":"tool_use"},{"type":"text"}]}}`, 0, 0, map[string]int{}, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tools := newToolCounts()
			var stamps []string
			res, err := scan(strings.NewReader(tt.text), tools, &stamps)
			if err != nil {
				t.Fatal(err)
			}
			var input float64
			for _, r := range res.rows {
				input += r.input
			}
			if len(res.rows) != tt.rows || input != tt.input || len(stamps) != tt.stampCount || res.secretReads != tt.secretReads {
				t.Errorf("rows %d input %v stamps %d secret %d; want %d %v %d %d", len(res.rows), input, len(stamps), res.secretReads, tt.rows, tt.input, tt.stampCount, tt.secretReads)
			}
			if len(tools.counts) != len(tt.tools) {
				t.Errorf("tools %v, want %v", tools.counts, tt.tools)
			}
			for name, n := range tt.tools {
				if tools.counts[name] != n {
					t.Errorf("tool %s = %d, want %d", name, tools.counts[name], n)
				}
			}
		})
	}
}

func TestScanCacheCreation(t *testing.T) {
	tests := []struct {
		name   string
		usage  string
		w5, w1 float64
	}{
		{"split object", `{"cache_creation":{"ephemeral_5m_input_tokens":3,"ephemeral_1h_input_tokens":4},"cache_creation_input_tokens":99,"input_tokens":1}`, 3, 4},
		{"flat field", `{"cache_creation_input_tokens":99,"input_tokens":1}`, 99, 0},
		{"empty split object falls back", `{"cache_creation":{},"cache_creation_input_tokens":5,"input_tokens":1}`, 5, 0},
		{"non-number counts are zero", `{"cache_creation":{"ephemeral_5m_input_tokens":"3"},"input_tokens":"x"}`, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stamps []string
			res, err := scan(strings.NewReader(`{"type":"assistant","message":{"id":"a","usage":`+tt.usage+`}}`), newToolCounts(), &stamps)
			if err != nil || len(res.rows) != 1 {
				t.Fatalf("rows %d err %v", len(res.rows), err)
			}
			if res.rows[0].write5m != tt.w5 || res.rows[0].write1h != tt.w1 {
				t.Errorf("writes %v %v, want %v %v", res.rows[0].write5m, res.rows[0].write1h, tt.w5, tt.w1)
			}
		})
	}
}

func TestPrices(t *testing.T) {
	p := mustPrices(t)
	if p.TableLabel() != "jitso/prices@2026-11-13" {
		t.Errorf("label %q", p.TableLabel())
	}
	if price, err := p.For("haiku"); err != nil || price.Output != 0.5 {
		t.Errorf("haiku %+v %v", price, err)
	}
	if _, err := p.For("mystery"); err == nil {
		t.Error("an unpriced family must be an error")
	}
	missing, _ := ParsePrices([]byte(`{"models":{"sonnet":{"input":1}}}`))
	if missing.VerifiedOn != "undefined" {
		t.Errorf("a missing verified_on reads %q, want undefined", missing.VerifiedOn)
	}
	if _, err := missing.For("sonnet"); err == nil {
		t.Error("a price entry without all five numbers must be an error")
	}
	for _, bad := range []string{"", "[]", "{", "null"} {
		if _, err := ParsePrices([]byte(bad)); err == nil {
			t.Errorf("ParsePrices(%q) must fail", bad)
		}
	}
}

func TestVerdictOf(t *testing.T) {
	check := func(names ...string) []catalog.SuccessCheck {
		var out []catalog.SuccessCheck
		for _, n := range names {
			out = append(out, catalog.SuccessCheck{Check: n})
		}
		return out
	}
	tests := []struct {
		name    string
		reads   int
		checks  []catalog.SuccessCheck
		verdict string
		reason  string
	}{
		{"secret read fails", 2, check("no_rule_breaks"), "fail", "a tool call read a secret path"},
		{"no checks is unknown", 0, nil, "unknown", "preset success checks 0 of 0 evaluated here; the rest need the destination"},
		{"only no_rule_breaks is success", 0, check("no_rule_breaks"), "success", "preset success checks 1 of 1 evaluated here; the rest need the destination"},
		{"one decidable of two is unknown", 0, check("no_rule_breaks", "answer_delivered"), "unknown", "preset success checks 1 of 2 evaluated here; the rest need the destination"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, r := verdictOf(tt.reads, tt.checks)
			if v != tt.verdict || r != tt.reason {
				t.Errorf("got %q %q, want %q %q", v, r, tt.verdict, tt.reason)
			}
		})
	}
}

// runEnv builds a project folder with prices, a registry holding one launch and
// a transcript, and the Deps over it.
func runEnv(t *testing.T, launchExtra string, withTranscript bool) (Deps, *registry.FileStore, string) {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, "jitso", "prices.json"), pricesJSON)
	transcript := filepath.Join(root, ".jitso", "transcripts", "t.jsonl")
	if withTranscript {
		write(t, transcript, mainTranscript)
	}
	store := registry.NewFileStore(filepath.Join(root, ".jitso"))
	launch := `{"v":1,"event":"launch","id":"L20261113T142233-a3f9","parent":null,"root":"L20261113T142233-a3f9","depth":0,"ts":"2026-11-13T14:22:33.000Z","mode":"direct","preset":"p","orca":{"terminal":"t"},"transcript_path":` + jsonString(transcript) + launchExtra + "}\n"
	write(t, store.Path(), launch)
	d := Deps{
		PricesPath: filepath.Join(root, "jitso", "prices.json"),
		Store:      store,
		Catalog: func() (*catalog.Catalog, error) {
			return &catalog.Catalog{Builds: []catalog.Build{{ID: "p", Success: []catalog.SuccessCheck{{Check: "no_rule_breaks"}}}}}, nil
		},
		Now: func() time.Time { return time.Date(2026, 11, 14, 1, 2, 3, 456e6, time.UTC) },
	}
	return d, store, root
}

func jsonString(s string) string { b, _ := json.Marshal(s); return string(b) }

func TestRunByID(t *testing.T) {
	d, store, _ := runEnv(t, `,"budget":{"tokens":250000}`, true)
	const id = "L20261113T142233-a3f9"
	out, err := Run([]string{id}, d)
	if err != nil {
		t.Fatal(err)
	}
	want := "jitso: analyzed " + id + ": 3 turns, peak 3105000 tokens, US$ 2.2 list, verdict fail (2 secret-path read(s))"
	if out != want {
		t.Errorf("stdout %q, want %q", out, want)
	}
	events, _ := store.Events()
	if len(events) != 3 || events[1].Kind() != registry.KindRuleBreak || events[2].Kind() != registry.KindAnalyzed {
		t.Fatalf("events %v", events)
	}
	if ev, _ := events[1].String("evidence"); ev != "2 tool call(s) touched a secret path" {
		t.Errorf("evidence %q", ev)
	}
	if pct, _ := events[2].Number("budget_pct"); pct != 1242 {
		t.Errorf("budget_pct %v, want 1242", pct)
	}
	if ts, _ := events[2].String("ts"); ts != "2026-11-14T01:02:03.456Z" {
		t.Errorf("ts %q", ts)
	}
	if table, _ := events[2].String("price_table"); table != "jitso/prices@2026-11-13" {
		t.Errorf("price_table %q", table)
	}
	// A second run appends a second analyzed event but no second rule_break.
	if _, err := Run([]string{id}, d); err != nil {
		t.Fatal(err)
	}
	events, _ = store.Events()
	if len(events) != 4 || events[3].Kind() != registry.KindAnalyzed {
		t.Errorf("second run: %d events", len(events))
	}
	raw, _ := os.ReadFile(store.Path())
	if strings.Contains(string(raw), ".env.fake") || strings.Contains(string(raw), "README") {
		t.Error("the registry must not hold a path or command from the transcript")
	}
}

func TestRunByIDBudget(t *testing.T) {
	tests := []struct {
		name   string
		extra  string
		absent bool
	}{
		{"none", ``, true},
		{"null", `,"budget":null`, true},
		{"empty", `,"budget":{}`, true},
		{"zero", `,"budget":{"tokens":0}`, true},
		{"string", `,"budget":{"tokens":"250000"}`, true},
		{"usable", `,"budget":{"tokens":100}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, store, _ := runEnv(t, tt.extra, true)
			if _, err := Run([]string{"L20261113T142233-a3f9"}, d); err != nil {
				t.Fatal(err)
			}
			events, _ := store.Events()
			last := events[len(events)-1]
			_, has := last.Get("budget_pct")
			if has == tt.absent {
				t.Errorf("budget_pct present = %v", has)
			}
		})
	}
}

func TestRunRejections(t *testing.T) {
	const id = "L20261113T142233-a3f9"
	tests := []struct {
		name       string
		args       []string
		transcript bool
		code       int
		text       string
	}{
		{"no args", nil, true, 2, Usage},
		{"extra arg", []string{id, "x"}, true, 2, Usage},
		{"bad id", []string{"../x"}, true, 2, Usage},
		{"upper-case hex", []string{"L20261113T142233-A3F9"}, true, 2, Usage},
		{"transcript without file", []string{"--transcript"}, true, 2, Usage},
		{"transcript twice", []string{"--transcript", "a.jsonl", "b.jsonl"}, true, 2, Usage},
		{"transcript wrong suffix", []string{"--transcript", "a.json"}, true, 2, Usage},
		{"transcript missing", []string{"--transcript", "/nonexistent/x.jsonl"}, true, 2, "x.jsonl does not exist"},
		{"unknown launch", []string{"L20990101T000000-ffff"}, true, 2, "unknown launch L20990101T000000-ffff"},
		{"no transcript on disk", []string{id}, false, 2, "launch " + id + " has no transcript on disk yet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, store, _ := runEnv(t, "", tt.transcript)
			out, err := Run(tt.args, d)
			if fail.Code(err) != tt.code || !strings.Contains(fail.Text(err), tt.text) || out != "" {
				t.Errorf("code %d text %q out %q, want %d %q", fail.Code(err), fail.Text(err), out, tt.code, tt.text)
			}
			if events, _ := store.Events(); len(events) != 1 {
				t.Errorf("a rejected run wrote %d events", len(events)-1)
			}
		})
	}
}

func TestRunPricesFirst(t *testing.T) {
	d, _, _ := runEnv(t, "", true)
	d.PricesPath = filepath.Join(t.TempDir(), "missing.json")
	_, err := Run([]string{"../x"}, d)
	var fe *fail.Error
	if err == nil || errors.As(err, &fe) || fail.Code(err) != 3 {
		t.Errorf("a missing prices file is a plain exit-3 error even for a bad id, got %v", err)
	}
}

func TestRunTranscriptMode(t *testing.T) {
	d, store, root := runEnv(t, "", true)
	out, err := Run([]string{"--transcript", filepath.Join(root, ".jitso", "transcripts", "t.jsonl")}, d)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "{\n \"session\": \"t\",\n \"files\": 1,") {
		t.Errorf("the JSON is indented with one space, got %.60q", out)
	}
	for _, want := range []string{`"secret_path_reads": 2`, `"max_main_context_tokens": 3105000`, `"cost_usd_list": 2.2`} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %s", want)
		}
	}
	if strings.Contains(out, "README") || strings.Contains(out, ".env.fake") {
		t.Error("the output must not hold a path or command from the transcript")
	}
	if events, _ := store.Events(); len(events) != 1 {
		t.Error("transcript mode must write nothing")
	}
}
