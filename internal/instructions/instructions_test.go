package instructions

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderShape(t *testing.T) {
	for _, unit := range []string{"\n", "\r\n"} {
		got := Render(AddedNew, unit)
		lines := strings.Split(got, unit)
		if lines[0] != `<!-- BEGIN JITSO block=2 added=new: managed by "jitso setup instructions"; edits inside this block are replaced -->` {
			t.Errorf("BEGIN line %q", lines[0])
		}
		if lines[len(lines)-1] != "<!-- END JITSO block=2 -->" {
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
		name   string
		data   string
		target string
		want   bool
	}{
		{"own line", "@AGENTS.md\n", "AGENTS.md", true},
		{"after text", "# Claude\n\nSee @AGENTS.md for the rules.\n", "AGENTS.md", true},
		{"dot slash", "@./AGENTS.md\n", "AGENTS.md", true},
		{"crlf", "@AGENTS.md\r\n", "AGENTS.md", true},
		{"no final break", "x\n@AGENTS.md", "AGENTS.md", true},
		{"code span", "Use `@AGENTS.md` to import.\n", "AGENTS.md", false},
		{"double backtick span", "Use ``@AGENTS.md`` here.\n", "AGENTS.md", false},
		{"span closes after", "`a` and @AGENTS.md\n", "AGENTS.md", true},
		{"unmatched backtick", "` @AGENTS.md\n", "AGENTS.md", true},
		{"fenced", "```\n@AGENTS.md\n```\n", "AGENTS.md", false},
		{"tilde fenced", "~~~\n@AGENTS.md\n~~~\n", "AGENTS.md", false},
		{"after a closed fence", "```\nx\n```\n@AGENTS.md\n", "AGENTS.md", true},
		{"other file", "@docs/AGENTS.md\n", "AGENTS.md", false},
		{"inside a word", "mail@AGENTS.md\n", "AGENTS.md", false},
		{"trailing period", "@AGENTS.md.\n", "AGENTS.md", false},
		{"empty", "", "AGENTS.md", false},
		{"nothing", "# Claude\n", "AGENTS.md", false},
		{"parent from .claude", "@../AGENTS.md\n", "../AGENTS.md", true},
		{"parent with dot slash", "@./../AGENTS.md\n", "../AGENTS.md", true},
		{"same folder from .claude", "@AGENTS.md\n", "../AGENTS.md", false},
		{"parent from the checkout", "@../AGENTS.md\n", "AGENTS.md", false},
	}
	for _, c := range cases {
		if got := Imports([]byte(c.data), c.target); got != c.want {
			t.Errorf("%s: Imports(%q, %q) = %v, want %v", c.name, c.data, c.target, got, c.want)
		}
	}
}

// memEnv is an in-memory machine for the target lists.
type memEnv map[string]string

func (m memEnv) env(root, home, codex string) Env {
	return Env{
		Root: root, Home: home, CodexHome: codex,
		ReadFile: func(p string) ([]byte, error) {
			p = filepath.ToSlash(p)
			if v, ok := m[p]; ok {
				return []byte(v), nil
			}
			return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
		},
		IsDir: func(p string) bool {
			p = filepath.ToSlash(p)
			for k := range m {
				if strings.HasPrefix(k, p+"/") {
					return true
				}
			}
			return m[p+"/"] == "dir"
		},
	}
}

// links makes EvalSymlinks of an Env follow the given links: each key is a
// link path (a file or a folder) and its value the absolute path it leads to.
// A path whose file is absent (not in files) fails, as the real one does.
func links(e Env, files memEnv, l map[string]string) Env {
	e.EvalSymlinks = func(p string) (string, error) {
		p = filepath.ToSlash(p)
		for {
			changed := false
			for from, to := range l {
				if p == from || strings.HasPrefix(p, from+"/") {
					p = to + strings.TrimPrefix(p, from)
					changed = true
				}
			}
			if !changed {
				break
			}
		}
		if _, ok := files[p]; ok || e.IsDir(p) || p == e.Root || p == e.Home {
			return filepath.FromSlash(p), nil
		}
		return "", &fs.PathError{Op: "lstat", Path: p, Err: fs.ErrNotExist}
	}
	return e
}

func displays(ts []Target) []string {
	var out []string
	for _, t := range ts {
		d := t.Display
		if t.Legacy {
			d += "(legacy)"
		}
		out = append(out, d)
	}
	return out
}

func eq(a, b []string) bool { return strings.Join(a, "|") == strings.Join(b, "|") }

func TestProjectTargets(t *testing.T) {
	warn := func(rel, imp, end string) string {
		return "jitso: warning: " + rel + " does not import AGENTS.md, and Claude Code reads only " + rel + " when it exists: add a line @" + imp + " to it" + end
	}
	const noBlock, keeps = " so Claude Code reads the JITSO block", "; its old JITSO block stays until then"
	oldBlock := Render(Added1, "\n") + "\n"
	cases := []struct {
		name   string
		files  map[string]string
		remove bool
		want   []string
		warns  []string
	}{
		{"agents only", nil, false, []string{"AGENTS.md"}, nil},
		{"claude without the import", map[string]string{"CLAUDE.md": "# c\n"}, false, []string{"AGENTS.md"},
			[]string{warn("CLAUDE.md", "AGENTS.md", noBlock)}},
		{"claude without the import keeps its block", map[string]string{"CLAUDE.md": "# c\n\n" + oldBlock}, false, []string{"AGENTS.md"},
			[]string{warn("CLAUDE.md", "AGENTS.md", keeps)}},
		{"claude that imports is legacy", map[string]string{"CLAUDE.md": "@AGENTS.md\n"}, false, []string{"AGENTS.md", "CLAUDE.md(legacy)"}, nil},
		{"dot claude imports from its parent", map[string]string{".claude/CLAUDE.md": "@../AGENTS.md\n"}, false, []string{"AGENTS.md", ".claude/CLAUDE.md(legacy)"}, nil},
		{"dot claude with the checkout import", map[string]string{".claude/CLAUDE.md": "@AGENTS.md\n"}, false, []string{"AGENTS.md"},
			[]string{warn(".claude/CLAUDE.md", "../AGENTS.md", noBlock)}},
		{"both claude files", map[string]string{"CLAUDE.md": "# c\n", ".claude/CLAUDE.md": "@../AGENTS.md\n"}, false, []string{"AGENTS.md", ".claude/CLAUDE.md(legacy)"},
			[]string{warn("CLAUDE.md", "AGENTS.md", noBlock)}},
		{"claude local is not looked at", map[string]string{"CLAUDE.local.md": "# mine\n"}, false, []string{"AGENTS.md"}, nil},
		{"remove lists every claude file", map[string]string{"CLAUDE.md": "@AGENTS.md\n", ".claude/CLAUDE.md": "# d\n"}, true, []string{"AGENTS.md", "CLAUDE.md(legacy)", ".claude/CLAUDE.md(legacy)"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := memEnv{}
			for k, v := range c.files {
				m["/p/"+k] = v
			}
			got, err := ProjectTargets(m.env("/p", "", ""), c.remove)
			if err != nil {
				t.Fatal(err)
			}
			if !eq(displays(got.List), c.want) {
				t.Errorf("targets = %v, want %v", displays(got.List), c.want)
			}
			if !eq(got.Warnings, c.warns) {
				t.Errorf("warnings = %v, want %v", got.Warnings, c.warns)
			}
			if len(got.Notes) != 0 || got.None != "" {
				t.Errorf("notes %v, none %q", got.Notes, got.None)
			}
			if got.List[0].Old != nil {
				t.Errorf("AGENTS.md should be absent")
			}
			for _, tg := range got.List {
				if want := map[string]string{"CLAUDE.md": "AGENTS.md", ".claude/CLAUDE.md": "../AGENTS.md"}[tg.Display]; tg.Import != want {
					t.Errorf("%s: import %q, want %q", tg.Display, tg.Import, want)
				}
			}
		})
	}
}

func TestProjectTargetsLinks(t *testing.T) {
	// CLAUDE.md is a link to AGENTS.md: one file, named AGENTS.md.
	m := memEnv{"/p/AGENTS.md": "# a\n", "/p/CLAUDE.md": "# a\n"}
	got, err := ProjectTargets(links(m.env("/p", "", ""), m, map[string]string{"/p/CLAUDE.md": "/p/AGENTS.md"}), false)
	if err != nil {
		t.Fatal(err)
	}
	if !eq(displays(got.List), []string{"AGENTS.md"}) || !eq(got.Notes, []string{"note: CLAUDE.md is the same file as AGENTS.md"}) {
		t.Errorf("targets %v, notes %v", displays(got.List), got.Notes)
	}

	// AGENTS.md leads out of the checkout: skipped with a warning, and the
	// add then lists nothing, an old block in a CLAUDE.md that imports it
	// included.
	m = memEnv{"/p/AGENTS.md": "# a\n", "/elsewhere/AGENTS.md": "# a\n", "/p/CLAUDE.md": "@AGENTS.md\n"}
	e := links(m.env("/p", "", ""), m, map[string]string{"/p/AGENTS.md": "/elsewhere/AGENTS.md"})
	got, err = ProjectTargets(e, false)
	if err != nil {
		t.Fatal(err)
	}
	wantWarn := "jitso: warning: AGENTS.md is a link to " + filepath.FromSlash("/elsewhere/AGENTS.md") + ", which is outside this checkout, so jitso left it alone"
	if len(got.List) != 0 || !eq(got.Warnings, []string{wantWarn}) || got.None != "no AGENTS.md jitso may write" {
		t.Errorf("targets %v, warnings %v, none %q", displays(got.List), got.Warnings, got.None)
	}
	// Removing still cleans CLAUDE.md.
	got, err = ProjectTargets(e, true)
	if err != nil {
		t.Fatal(err)
	}
	if !eq(displays(got.List), []string{"CLAUDE.md(legacy)"}) || got.None != "" {
		t.Errorf("remove: targets %v, none %q", displays(got.List), got.None)
	}

	// A linked folder: the file is written under the scope root.
	m = memEnv{"/p/real/CLAUDE.md": "@../AGENTS.md\n", "/p/.claude/CLAUDE.md": "@../AGENTS.md\n", "/p/AGENTS.md": ""}
	got, err = ProjectTargets(links(m.env("/p", "", ""), m, map[string]string{"/p/.claude": "/p/real"}), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.List) != 2 || got.List[1].Path != filepath.FromSlash("/p/real/CLAUDE.md") || got.List[1].Display != ".claude/CLAUDE.md" {
		t.Errorf("targets = %+v", got.List)
	}
}

func TestUserTargets(t *testing.T) {
	const noFolder = "no harness user folder found: ~/.codex, ~/.config/opencode, ~/.pi/agent"
	const noOpencode = "jitso: warning: ~/.config/opencode/AGENTS.md was not created, because OpenCode reads ~/.claude/CLAUDE.md only while it is absent; create it yourself (it may import your rules) and run again"
	cases := []struct {
		name   string
		files  map[string]string
		codex  string
		remove bool
		want   []string
		warns  []string
		none   string
	}{
		{"empty home", nil, "", false, nil, nil, noFolder},
		{"old block with no harness stays", map[string]string{"/h/.claude/CLAUDE.md": "x\n"}, "", false, nil, nil, noFolder},
		{"codex folder", map[string]string{"/h/.codex/config.toml": "x"}, "", false, []string{"~/.codex/AGENTS.md"}, nil, ""},
		{"opencode folder", map[string]string{"/h/.config/opencode/opencode.json": "{}"}, "", false, []string{"~/.config/opencode/AGENTS.md"}, nil, ""},
		{"pi folder", map[string]string{"/h/.pi/agent/settings.json": "{}"}, "", false, []string{"~/.pi/agent/AGENTS.md"}, nil, ""},
		{"every harness, and claude is not listed", map[string]string{
			"/h/.codex/config.toml": "x", "/h/.config/opencode/AGENTS.md": "# oc\n", "/h/.pi/agent/settings.json": "{}", "/h/.claude/CLAUDE.md": "x\n",
		}, "", false, []string{"~/.codex/AGENTS.md", "~/.config/opencode/AGENTS.md", "~/.pi/agent/AGENTS.md"}, nil, ""},
		{"opencode is not created beside claude", map[string]string{
			"/h/.config/opencode/opencode.json": "{}", "/h/.pi/agent/settings.json": "{}", "/h/.claude/CLAUDE.md": "x\n",
		}, "", false, []string{"~/.pi/agent/AGENTS.md"}, []string{noOpencode}, ""},
		{"opencode alone beside claude", map[string]string{
			"/h/.config/opencode/opencode.json": "{}", "/h/.claude/CLAUDE.md": "x\n",
		}, "", false, nil, []string{noOpencode}, "no AGENTS.md jitso may write"},
		{"override with text", map[string]string{"/h/.codex/AGENTS.override.md": "x\n", "/h/.pi/agent/settings.json": "{}"}, "", false, []string{"~/.pi/agent/AGENTS.md"},
			[]string{"jitso: warning: ~/.codex/AGENTS.override.md is not empty, so Codex reads it instead of AGENTS.md; the JITSO block was not written to ~/.codex/AGENTS.md"}, ""},
		{"override alone", map[string]string{"/h/.codex/AGENTS.override.md": "x\n"}, "", false, nil,
			[]string{"jitso: warning: ~/.codex/AGENTS.override.md is not empty, so Codex reads it instead of AGENTS.md; the JITSO block was not written to ~/.codex/AGENTS.md"}, "no AGENTS.md jitso may write"},
		{"empty override", map[string]string{"/h/.codex/AGENTS.override.md": ""}, "", false, []string{"~/.codex/AGENTS.md"}, nil, ""},
		{"blank override", map[string]string{"/h/.codex/AGENTS.override.md": " \n"}, "", false, []string{"~/.codex/AGENTS.md"}, nil, ""},
		{"codex home", map[string]string{"/h/alt/config.toml": "x"}, "/h/alt", false, []string{"~/alt/AGENTS.md"}, nil, ""},
		{"codex home folder missing", nil, "/h/alt", false, nil, nil, "no harness user folder found: ~/alt, ~/.config/opencode, ~/.pi/agent"},
		{"codex home outside the home folder", map[string]string{"/elsewhere/config.toml": "x"}, "/elsewhere", false, nil,
			[]string{"jitso: warning: CODEX_HOME (/elsewhere) is not inside your home folder, so the JITSO block was not written to its AGENTS.md"}, "no AGENTS.md jitso may write"},
		{"remove skips the override", map[string]string{"/h/.codex/AGENTS.override.md": "x\n", "/h/.codex/AGENTS.md": "a\n"}, "", true, []string{"~/.codex/AGENTS.md", "~/.claude/CLAUDE.md(legacy)"}, nil, ""},
		{"remove lists existing files only, claude always", map[string]string{"/h/.config/opencode/opencode.json": "{}", "/h/.pi/agent/AGENTS.md": "a\n"}, "", true, []string{"~/.pi/agent/AGENTS.md", "~/.claude/CLAUDE.md(legacy)"}, nil, ""},
		{"remove in an empty home", nil, "", true, []string{"~/.claude/CLAUDE.md(legacy)"}, nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := UserTargets(memEnv(c.files).env("", "/h", c.codex), c.remove)
			if err != nil {
				t.Fatal(err)
			}
			if !eq(displays(got.List), c.want) {
				t.Errorf("targets = %v, want %v", displays(got.List), c.want)
			}
			if !eq(got.Warnings, c.warns) {
				t.Errorf("warnings = %v, want %v", got.Warnings, c.warns)
			}
			if got.None != c.none {
				t.Errorf("none = %q, want %q", got.None, c.none)
			}
			for _, tg := range got.List {
				if tg.Legacy && (tg.PruneDir != filepath.FromSlash("/h/.claude") || tg.PruneDisplay != "~/.claude") {
					t.Errorf("prune = %q %q", tg.PruneDir, tg.PruneDisplay)
				}
				if !tg.Legacy && tg.PruneDir != "" {
					t.Errorf("%s prunes %q", tg.Display, tg.PruneDir)
				}
			}
		})
	}
}

func TestUserTargetsLinks(t *testing.T) {
	m := memEnv{
		"/h/.codex/config.toml":         "x",
		"/h/.agents/AGENTS.md":          "# shared\n",
		"/h/.codex/AGENTS.md":           "# shared\n",
		"/h/.pi/agent/AGENTS.md":        "# shared\n",
		"/h/.claude/CLAUDE.md":          "# shared\n",
		"/out/AGENTS.md":                "# out\n",
		"/h/.config/opencode/AGENTS.md": "# out\n",
	}
	e := links(m.env("", "/h", ""), m, map[string]string{
		"/h/.codex/AGENTS.md":           "/h/.agents/AGENTS.md",
		"/h/.pi/agent/AGENTS.md":        "/h/.agents/AGENTS.md",
		"/h/.claude/CLAUDE.md":          "/h/.agents/AGENTS.md",
		"/h/.config/opencode/AGENTS.md": "/out/AGENTS.md",
	})
	got, err := UserTargets(e, false)
	if err != nil {
		t.Fatal(err)
	}
	if !eq(displays(got.List), []string{"~/.codex/AGENTS.md"}) || got.List[0].Path != filepath.FromSlash("/h/.agents/AGENTS.md") {
		t.Errorf("targets = %+v", got.List)
	}
	wantNotes := []string{"note: ~/.pi/agent/AGENTS.md is the same file as ~/.codex/AGENTS.md"}
	if !eq(got.Notes, wantNotes) {
		t.Errorf("notes = %v", got.Notes)
	}
	wantWarn := []string{"jitso: warning: ~/.config/opencode/AGENTS.md is a link to " + filepath.FromSlash("/out/AGENTS.md") + ", which is outside your home folder, so jitso left it alone"}
	if !eq(got.Warnings, wantWarn) {
		t.Errorf("warnings = %v", got.Warnings)
	}

	// Removing lists ~/.claude/CLAUDE.md too: the same file again.
	got, err = UserTargets(e, true)
	if err != nil {
		t.Fatal(err)
	}
	wantNotes = append(wantNotes, "note: ~/.claude/CLAUDE.md is the same file as ~/.codex/AGENTS.md")
	if !eq(displays(got.List), []string{"~/.codex/AGENTS.md"}) || !eq(got.Notes, wantNotes) || !eq(got.Warnings, wantWarn) {
		t.Errorf("remove: targets %v, notes %v, warnings %v", displays(got.List), got.Notes, got.Warnings)
	}
}

func TestReadErrorIsAnEnvironmentError(t *testing.T) {
	e := Env{Root: "/p", ReadFile: func(string) ([]byte, error) { return nil, errors.New("boom") }}
	if _, err := ProjectTargets(e, false); err == nil || !strings.Contains(err.Error(), "could not read") {
		t.Errorf("err = %v", err)
	}
}
