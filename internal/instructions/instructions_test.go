package instructions

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
)

func TestRenderShape(t *testing.T) {
	for _, unit := range []string{"\n", "\r\n"} {
		got := Render(AddedNew, unit)
		lines := strings.Split(got, unit)
		if lines[0] != `<!-- BEGIN JITSO block=1 added=new: managed by "jitso setup instructions"; edits inside this block are replaced -->` {
			t.Errorf("BEGIN line %q", lines[0])
		}
		if lines[len(lines)-1] != "<!-- END JITSO block=1 -->" {
			t.Errorf("END line %q", lines[len(lines)-1])
		}
		if strings.HasSuffix(got, unit) {
			t.Errorf("the block has a break after END")
		}
		if unit == "\r\n" && strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
			t.Errorf("a bare LF in a CRLF block")
		}
	}
}

// The block text is shipped prose: no dash characters, no import-looking @,
// no counts or dates.
func TestBlockTextRules(t *testing.T) {
	text := Render(Added1, "\n")
	for _, bad := range []string{"\u2014", "\u2013", "@"} {
		if strings.Contains(text, bad) {
			t.Errorf("block text holds %q", bad)
		}
	}
	if len(scan([]byte(text)).markers) != 2 {
		t.Errorf("the rendered block must hold exactly its two markers")
	}
}

func TestInsertTable(t *testing.T) {
	cases := []struct {
		name  string
		old   []byte
		unit  string
		lead  string
		added string
	}{
		{"absent", nil, "\n", "", AddedNew},
		{"empty", []byte{}, "\n", "", Added0},
		{"trailing LF", []byte("a\n"), "\n", "\n", Added1},
		{"no trailing break", []byte("a"), "\n", "\n\n", Added2},
		{"blank line at the end", []byte("a\n\n"), "\n", "\n", Added1},
		{"trailing CRLF", []byte("a\r\n"), "\r\n", "\r\n", Added1},
		{"CRLF no trailing break", []byte("a\r\nb"), "\r\n", "\r\n\r\n", Added2},
		{"LF file that ends in CRLF", []byte("a\nb\r\n"), "\n", "\n", Added1},
		{"lone CR at the end, no LF", []byte("a\r"), "\r\n", "\r\n\r\n", Added2},
		{"lone CR in the middle", []byte("a\rb"), "\n", "\n\n", Added2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			want := string(c.old) + c.lead + Render(c.added, c.unit) + c.unit
			got := Insert(c.old)
			if string(got) != want {
				t.Fatalf("Insert =\n%q\nwant\n%q", got, want)
			}
			b, err := Find(got)
			if err != nil || b == nil {
				t.Fatalf("Find: %v %v", b, err)
			}
			if b.Added != c.added || b.Number != Number {
				t.Errorf("block = %+v", b)
			}
		})
	}
}

func TestRemoveRoundTrips(t *testing.T) {
	originals := [][]byte{
		nil, {}, []byte("\n"), []byte("a"), []byte("a\n"), []byte("a\n\n"), []byte("a\n\n\n"),
		[]byte("a\r\n"), []byte("a\r\nb"), []byte("a\r\n\r\n"), []byte("a\nb\r\n"), []byte("a\r"),
		[]byte("a\rb"), []byte("\r\n"), []byte("# T\n\n```\n<!-- BEGIN JITSO block=9 added=0 -->\nx\n<!-- END JITSO block=9 -->\n```\n"),
		[]byte("~~~\n<!-- BEGIN JITSO block=9 added=0 -->\n~~~\n"),
		[]byte("# T\n\n````md\n```\n<!-- END JITSO block=9 -->\n````\n"),
	}
	for _, old := range originals {
		with := Insert(old)
		res, err := Remove(with)
		if err != nil || !res.Found {
			t.Fatalf("Remove(%q): %+v %v", old, res, err)
		}
		if old == nil {
			if !res.Delete || len(res.Out) != 0 {
				t.Errorf("an absent original must come back as a deleted file, got %+v", res)
			}
			continue
		}
		if res.Delete {
			t.Errorf("original %q: Delete without added=new", old)
		}
		if string(res.Out) != string(old) {
			t.Errorf("original %q: Remove gives %q", old, res.Out)
		}
		// A second insert is the same bytes again.
		if again := Insert(res.Out); string(again) != string(with) {
			t.Errorf("original %q: insert after remove differs", old)
		}
	}
}

func TestRemoveKeepsWhatFollows(t *testing.T) {
	data := []byte("a\n\n" + Render(Added1, "\n") + "\n\nlater\n")
	res, err := Remove(data)
	if err != nil || string(res.Out) != "a\n\nlater\n" {
		t.Fatalf("Remove = %q, %v", res.Out, err)
	}
}

func TestRemoveBlockMovedByHand(t *testing.T) {
	// added=1 but the block was moved to the top: nothing before it to take.
	data := []byte(Render(Added1, "\n") + "\nrest\n")
	res, _ := Remove(data)
	if string(res.Out) != "rest\n" || res.Delete {
		t.Errorf("Remove = %+v", res)
	}
	// added=0 in the middle: only its own lines.
	mid := []byte("a\n\n<!-- BEGIN JITSO block=0 added=0 -->\nold\n<!-- END JITSO block=0 -->\n\nb\n")
	res, _ = Remove(mid)
	if string(res.Out) != "a\n\n\nb\n" {
		t.Errorf("Remove(mid) = %q", res.Out)
	}
	// added=new with text written before it stays a file.
	nw := []byte("mine\n" + Render(AddedNew, "\n") + "\n")
	res, _ = Remove(nw)
	if res.Delete || string(res.Out) != "mine\n" {
		t.Errorf("Remove(new with text) = %+v", res)
	}
}

func TestRemoveWithoutBlock(t *testing.T) {
	res, err := Remove([]byte("nothing here\n"))
	if err != nil || res.Found || string(res.Out) != "nothing here\n" {
		t.Errorf("Remove = %+v, %v", res, err)
	}
}

func TestEnsure(t *testing.T) {
	// Nothing to do on a current block, whatever its added.
	for _, old := range [][]byte{nil, {}, []byte("a\n"), []byte("a"), []byte("a\r\n")} {
		first, err := Ensure(old)
		if err != nil {
			t.Fatal(err)
		}
		second, err := Ensure(first)
		if err != nil || string(second) != string(first) {
			t.Errorf("Ensure is not stable for %q", old)
		}
	}
	// A block with an older number or no attributes is replaced in place and
	// keeps its added value.
	old := "# A\n\n<!-- BEGIN JITSO block=0 added=1 -->\nold\n<!-- END JITSO block=0 -->\n\n# After\n"
	got, err := Ensure([]byte(old))
	if err != nil {
		t.Fatal(err)
	}
	want := "# A\n\n" + Render(Added1, "\n") + "\n\n# After\n"
	if string(got) != want {
		t.Errorf("Ensure =\n%q\nwant\n%q", got, want)
	}
	// A BEGIN with no attributes counts as block=0 added=0.
	bare := "<!-- BEGIN JITSO -->\nx\n<!-- END JITSO -->\n"
	got, _ = Ensure([]byte(bare))
	if string(got) != Render(Added0, "\n")+"\n" {
		t.Errorf("Ensure(bare) = %q", got)
	}
	// The END line's own break is kept (CRLF in a file whose unit is LF).
	mixed := "x\n" + Render(Added1, "\n") + "\r\n"
	if got, _ := Ensure([]byte(mixed)); string(got) != mixed {
		t.Errorf("a current block was rewritten: %q", got)
	}
	// A block written with another added value is current too.
	for _, added := range []string{AddedNew, Added0, Added1, Added2} {
		cur := []byte("x\n" + Render(added, "\n") + "\n")
		if got, _ := Ensure(cur); string(got) != string(cur) {
			t.Errorf("added=%s was rewritten", added)
		}
	}
}

func TestEnsureCRLFBlockInPlace(t *testing.T) {
	data := []byte("# A\r\n\r\n<!-- BEGIN JITSO block=0 added=1 -->\r\nold\r\n<!-- END JITSO block=0 -->\r\n# B\r\n")
	got, err := Ensure(data)
	if err != nil {
		t.Fatal(err)
	}
	want := "# A\r\n\r\n" + Render(Added1, "\r\n") + "\r\n# B\r\n"
	if string(got) != want {
		t.Errorf("Ensure =\n%q\nwant\n%q", got, want)
	}
}

func TestFenceSkipping(t *testing.T) {
	const b, e = "<!-- BEGIN JITSO block=1 added=1 -->", "<!-- END JITSO block=1 -->"
	cases := []struct {
		name  string
		data  string
		found bool
	}{
		{"backtick fence", "```\n" + b + "\nx\n" + e + "\n```\n", false},
		{"tilde fence", "~~~\n" + b + "\nx\n" + e + "\n~~~\n", false},
		{"longer fence holds shorter", "````\n```\n" + b + "\nx\n" + e + "\n```\n````\n", false},
		{"tilde does not close a backtick fence", "```\n~~~\n" + b + "\nx\n" + e + "\n```\n", false},
		{"fence info text", "```md\n" + b + "\nx\n" + e + "\n```\n", false},
		{"after a closed fence", "```\ncode\n```\n" + b + "\nx\n" + e + "\n", true},
		{"indented fence", "   ```\n" + b + "\nx\n" + e + "\n   ```\n", false},
		{"four spaces is not a fence", "    ```\n" + b + "\nx\n" + e + "\n", true},
		{"marker with leading space is not a marker", " " + b + "\nx\n " + e + "\n", false},
		{"inline backticks do not open a fence", "```not a fence```\n" + b + "\nx\n" + e + "\n", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Find([]byte(c.data))
			if err != nil {
				t.Fatal(err)
			}
			if (got != nil) != c.found {
				t.Errorf("found = %v, want %v", got != nil, c.found)
			}
		})
	}
}

func TestOpenFence(t *testing.T) {
	_, err := Ensure([]byte("# A\n\n```sh\necho hi\n"))
	var p *Problem
	if !errors.As(err, &p) || p.Kind != OpenFence || p.Line != 3 {
		t.Fatalf("err = %v", err)
	}
	if got := p.Message("AGENTS.md"); got != "AGENTS.md ends inside a code fence that starts at line 3; close the fence by hand; nothing written" {
		t.Errorf("Message = %q", got)
	}
	// A block found before the fence is still handled; the fence is not the
	// block's concern.
	ok := Render(Added1, "\n") + "\n```\nopen\n"
	if _, err := Ensure([]byte(ok)); err != nil {
		t.Errorf("a block before an open fence is fine: %v", err)
	}
	// Remove does not care about an open fence.
	if _, err := Remove([]byte("```\nx\n")); err != nil {
		t.Errorf("Remove: %v", err)
	}
}

func TestBrokenMarkers(t *testing.T) {
	const b, e = "<!-- BEGIN JITSO block=1 added=1 -->", "<!-- END JITSO block=1 -->"
	cases := []struct {
		name string
		data string
		kind ProblemKind
		line int
		text string
	}{
		{"begin only", "a\n" + b + "\nx\n", BeginWithoutEnd, 2, "a BEGIN without an END at line 2"},
		{"end only", "a\nb\n" + e + "\n", EndWithoutBegin, 3, "an END without a BEGIN at line 3"},
		{"end before begin", e + "\n" + b + "\nx\n" + e + "\n", EndBeforeBegin, 1, "an END before its BEGIN at line 1"},
		{"two blocks", b + "\n" + e + "\n" + b + "\n" + e + "\n", SecondBlock, 3, "a second block at line 3"},
		{"begin inside a block", b + "\n" + b + "\n" + e + "\n", BeginInsideBlock, 2, "a BEGIN inside a block at line 2"},
		{"end after a block", b + "\n" + e + "\n" + e + "\n", EndWithoutBegin, 3, "an END without a BEGIN at line 3"},
		{"CRLF file", "a\r\n" + b + "\r\n", BeginWithoutEnd, 2, "a BEGIN without an END at line 2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for name, fn := range map[string]func([]byte) error{
				"Find":   func(d []byte) error { _, err := Find(d); return err },
				"Ensure": func(d []byte) error { _, err := Ensure(d); return err },
				"Remove": func(d []byte) error { _, err := Remove(d); return err },
			} {
				err := fn([]byte(c.data))
				var p *Problem
				if !errors.As(err, &p) || p.Kind != c.kind || p.Line != c.line {
					t.Fatalf("%s: err = %v", name, err)
				}
				if p.Error() != c.text {
					t.Errorf("%s: Error = %q", name, p.Error())
				}
			}
			var p *Problem
			_, err := Find([]byte(c.data))
			errors.As(err, &p)
			want := "AGENTS.md has a broken JITSO block (" + c.text + "); fix or remove the marker lines by hand; nothing written"
			if got := p.Message("AGENTS.md"); got != want {
				t.Errorf("Message = %q", got)
			}
		})
	}
}

func TestMarkersInsideAFenceAreNotBroken(t *testing.T) {
	data := "```\n<!-- BEGIN JITSO block=1 added=1 -->\n```\n"
	b, err := Find([]byte(data))
	if err != nil || b != nil {
		t.Errorf("Find = %v, %v", b, err)
	}
}

func TestBeginAttributes(t *testing.T) {
	cases := []struct {
		begin string
		block int
		added string
	}{
		{"<!-- BEGIN JITSO block=1 added=1: text -->", 1, Added1},
		{"<!-- BEGIN JITSO block=12 added=new: text -->", 12, AddedNew},
		{"<!-- BEGIN JITSO -->", 0, Added0},
		{"<!-- BEGIN JITSO block=3 -->", 3, Added0},
		{"<!-- BEGIN JITSO added=2 -->", 0, Added2},
		{"<!-- BEGIN JITSO block=1 added=7: text -->", 1, Added0},
		{"<!-- BEGIN JITSO: block=5 added=2 -->", 0, Added0}, // after the colon is prose
	}
	for _, c := range cases {
		b, err := Find([]byte(c.begin + "\nx\n<!-- END JITSO -->\n"))
		if err != nil || b == nil || b.Number != c.block || b.Added != c.added {
			t.Errorf("%q: %+v %v", c.begin, b, err)
		}
	}
}

func TestImports(t *testing.T) {
	cases := []struct {
		name string
		data string
		want bool
	}{
		{"own line", "@AGENTS.md\n", true},
		{"after text", "# Claude\n\nSee @AGENTS.md for the rules.\n", true},
		{"dot slash", "@./AGENTS.md\n", true},
		{"crlf", "@AGENTS.md\r\n", true},
		{"no final break", "x\n@AGENTS.md", true},
		{"code span", "Use `@AGENTS.md` to import.\n", false},
		{"double backtick span", "Use ``@AGENTS.md`` here.\n", false},
		{"span closes after", "`a` and @AGENTS.md\n", true},
		{"unmatched backtick", "` @AGENTS.md\n", true},
		{"fenced", "```\n@AGENTS.md\n```\n", false},
		{"tilde fenced", "~~~\n@AGENTS.md\n~~~\n", false},
		{"after a closed fence", "```\nx\n```\n@AGENTS.md\n", true},
		{"other file", "@docs/AGENTS.md\n", false},
		{"inside a word", "mail@AGENTS.md\n", false},
		{"trailing period", "@AGENTS.md.\n", false},
		{"empty", "", false},
		{"nothing", "# Claude\n", false},
	}
	for _, c := range cases {
		if got := Imports([]byte(c.data)); got != c.want {
			t.Errorf("%s: Imports(%q) = %v, want %v", c.name, c.data, got, c.want)
		}
	}
}

// memEnv is an in-memory machine for the target lists.
type memEnv map[string]string

func (m memEnv) env(root, home, codex string) Env {
	return Env{
		Root: root, Home: home, CodexHome: codex,
		ReadFile: func(p string) ([]byte, error) {
			if v, ok := m[p]; ok {
				return []byte(v), nil
			}
			return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
		},
		IsDir: func(p string) bool {
			for k := range m {
				if strings.HasPrefix(k, p+"/") {
					return true
				}
			}
			return m[p+"/"] == "dir"
		},
	}
}

func displays(ts []Target) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.Display)
	}
	return out
}

func eq(a, b []string) bool { return strings.Join(a, "|") == strings.Join(b, "|") }

func TestProjectTargets(t *testing.T) {
	cases := []struct {
		name   string
		files  map[string]string
		remove bool
		want   []string
		warns  int
	}{
		{"agents only", nil, false, []string{"AGENTS.md"}, 0},
		{"claude without import", map[string]string{"CLAUDE.md": "# c\n"}, false, []string{"AGENTS.md", "CLAUDE.md"}, 0},
		{"claude with import", map[string]string{"CLAUDE.md": "@AGENTS.md\n"}, false, []string{"AGENTS.md"}, 0},
		{"claude with the import in a span", map[string]string{"CLAUDE.md": "`@AGENTS.md`\n"}, false, []string{"AGENTS.md", "CLAUDE.md"}, 0},
		{"dot claude", map[string]string{".claude/CLAUDE.md": "# c\n"}, false, []string{"AGENTS.md", ".claude/CLAUDE.md"}, 0},
		{"root claude wins over dot claude", map[string]string{"CLAUDE.md": "# c\n", ".claude/CLAUDE.md": "# d\n"}, false, []string{"AGENTS.md", "CLAUDE.md"}, 0},
		{"import in root claude hides dot claude", map[string]string{"CLAUDE.md": "@AGENTS.md\n", ".claude/CLAUDE.md": "# d\n"}, false, []string{"AGENTS.md"}, 0},
		{"local warns", map[string]string{"CLAUDE.local.md": "# mine\n"}, false, []string{"AGENTS.md"}, 1},
		{"local with import is quiet", map[string]string{"CLAUDE.local.md": "@AGENTS.md\n"}, false, []string{"AGENTS.md"}, 0},
		{"local next to claude is quiet", map[string]string{"CLAUDE.local.md": "# m\n", "CLAUDE.md": "@AGENTS.md\n"}, false, []string{"AGENTS.md"}, 0},
		{"remove lists every existing file", map[string]string{"CLAUDE.md": "@AGENTS.md\n", ".claude/CLAUDE.md": "# d\n"}, true, []string{"AGENTS.md", "CLAUDE.md", ".claude/CLAUDE.md"}, 0},
		{"remove has no warning", map[string]string{"CLAUDE.local.md": "# m\n"}, true, []string{"AGENTS.md"}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := memEnv{}
			for k, v := range c.files {
				m["/p/"+k] = v
			}
			ts, warns, err := ProjectTargets(m.env("/p", "", ""), c.remove)
			if err != nil {
				t.Fatal(err)
			}
			if !eq(displays(ts), c.want) {
				t.Errorf("targets = %v, want %v", displays(ts), c.want)
			}
			if len(warns) != c.warns {
				t.Errorf("warnings = %v", warns)
			}
			if c.warns == 1 && warns[0] != LocalWarning {
				t.Errorf("warning = %q", warns[0])
			}
			if ts[0].Old != nil {
				t.Errorf("AGENTS.md should be absent")
			}
		})
	}
}

func TestUserTargets(t *testing.T) {
	cases := []struct {
		name   string
		files  map[string]string
		codex  string
		remove bool
		want   []string
		warns  []string
	}{
		{"empty home", nil, "", false, []string{"~/.claude/CLAUDE.md"}, nil},
		{"codex folder", map[string]string{"/h/.codex/config.toml": "x"}, "", false, []string{"~/.claude/CLAUDE.md", "~/.codex/AGENTS.md"}, nil},
		{"opencode file", map[string]string{"/h/.config/opencode/AGENTS.md": "# oc\n"}, "", false, []string{"~/.claude/CLAUDE.md", "~/.config/opencode/AGENTS.md"}, nil},
		{"opencode folder only", map[string]string{"/h/.config/opencode/opencode.json": "{}"}, "", false, []string{"~/.claude/CLAUDE.md"}, nil},
		{"override with text", map[string]string{"/h/.codex/AGENTS.override.md": "x\n"}, "", false, []string{"~/.claude/CLAUDE.md"},
			[]string{"jitso: warning: ~/.codex/AGENTS.override.md is not empty, so Codex reads it instead of AGENTS.md; the JITSO block was not written to ~/.codex/AGENTS.md"}},
		{"empty override", map[string]string{"/h/.codex/AGENTS.override.md": ""}, "", false, []string{"~/.claude/CLAUDE.md", "~/.codex/AGENTS.md"}, nil},
		{"blank override", map[string]string{"/h/.codex/AGENTS.override.md": " \n"}, "", false, []string{"~/.claude/CLAUDE.md", "~/.codex/AGENTS.md"}, nil},
		{"codex home", map[string]string{"/h/alt/config.toml": "x"}, "/h/alt", false, []string{"~/.claude/CLAUDE.md", "~/alt/AGENTS.md"}, nil},
		{"codex home folder missing", nil, "/h/alt", false, []string{"~/.claude/CLAUDE.md"}, nil},
		{"codex home outside the home folder", map[string]string{"/elsewhere/config.toml": "x"}, "/elsewhere", false, []string{"~/.claude/CLAUDE.md"},
			[]string{"jitso: warning: CODEX_HOME (/elsewhere) is not inside your home folder, so the JITSO block was not written to its AGENTS.md"}},
		{"remove skips the override", map[string]string{"/h/.codex/AGENTS.override.md": "x\n", "/h/.codex/AGENTS.md": "a\n"}, "", true, []string{"~/.claude/CLAUDE.md", "~/.codex/AGENTS.md"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ts, warns, err := UserTargets(memEnv(c.files).env("", "/h", c.codex), c.remove)
			if err != nil {
				t.Fatal(err)
			}
			if !eq(displays(ts), c.want) {
				t.Errorf("targets = %v, want %v", displays(ts), c.want)
			}
			if !eq(warns, c.warns) {
				t.Errorf("warnings = %v, want %v", warns, c.warns)
			}
			if ts[0].PruneDir != "/h/.claude" || ts[0].PruneDisplay != "~/.claude" {
				t.Errorf("prune = %q %q", ts[0].PruneDir, ts[0].PruneDisplay)
			}
		})
	}
}

func TestReadErrorIsAnEnvironmentError(t *testing.T) {
	e := Env{Root: "/p", ReadFile: func(string) ([]byte, error) { return nil, errors.New("boom") }}
	if _, _, err := ProjectTargets(e, false); err == nil || !strings.Contains(err.Error(), "could not read") {
		t.Errorf("err = %v", err)
	}
}
