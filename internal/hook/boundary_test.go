package hook

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// fakeFS is a file system of links and plain entries. A link maps to its
// target text; a folder or file is a key with an empty target in dirs.
type fakeFS struct {
	links map[string]string
	dirs  map[string]bool
}

func (f fakeFS) ports() FS {
	return FS{
		LinkStat: func(p string) (bool, bool) {
			if _, ok := f.links[p]; ok {
				return true, true
			}
			return f.dirs[p], false
		},
		Readlink: func(p string) (string, error) {
			if t, ok := f.links[p]; ok {
				return t, nil
			}
			return "", errors.New("not a link")
		},
		EvalSymlinks: func(p string) (string, error) {
			// resolve every folder link on the way, like the real call
			parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
			cur := ""
			for _, part := range parts {
				cur += "/" + part
				for hop := 0; hop < 10; hop++ {
					t, ok := f.links[cur]
					if !ok {
						break
					}
					if !filepath.IsAbs(t) {
						t = filepath.Join(filepath.Dir(cur), t)
					}
					cur = t
				}
			}
			return cur, nil
		},
	}
}

func newBoundary(f fakeFS) *Boundary {
	return &Boundary{Dir: "/p", Home: "/home/me", Primary: func() string { return "/main" }, FS: f.ports()}
}

func TestArea(t *testing.T) {
	for path, want := range map[string]string{
		"/p/jitso/builds/x.yaml":         areaBuilds,
		"/p/jitso/builds":                areaBuilds,
		"/p/jitso/builds/sub/x.yaml":     areaBuilds,
		"/p/JITSO/Builds/x.yaml":         areaBuilds,
		"/p/jitso/hard-rules/core.md":    areaRules,
		"/p/jitso/hard-rules":            areaRules,
		"/p/jitso/jitso.yaml":            areaYAML,
		"/p/Jitso/JITSO.YAML":            areaYAML,
		"/a/jitso/jitso/builds/x":        areaBuilds,
		"/p/jitso/jitso.yaml/x":          "",
		"/p/jitso/jitso.yaml.bak":        "",
		"/p/jitso/builds-notes.md":       "",
		"/p/jitso/dispatcher/prompt.md":  "",
		"/p/docs/jitso.yaml":             "",
		"/p/.jitso/drafts/x.yaml":        "",
		"/p/src/builds/x.ts":             "",
		"/p/jitso":                       "",
		"/p/docs/../jitso/builds/x.yaml": areaBuilds,
		"/p/jitsos/builds/x.yaml":        "",
	} {
		if got := Area(path); got != want {
			t.Errorf("Area(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestPathArea(t *testing.T) {
	f := fakeFS{
		links: map[string]string{
			"/p/notes/b":        "/p/jitso/builds",
			"/p/notes/rules.md": "/p/jitso/hard-rules/core.md",
			"/p/notes/new.yaml": "/p/jitso/builds/new.yaml",
			"/p/notes/rel":      "../jitso/jitso.yaml",
			"/p/notes/loop1":    "/p/notes/loop2",
			"/p/notes/loop2":    "/p/notes/loop1",
			"/p/notes/chain":    "/p/notes/b",
		},
		dirs: map[string]bool{"/p": true, "/p/notes": true, "/p/jitso": true, "/p/jitso/builds": true, "/p/jitso/hard-rules": true},
	}
	b := newBoundary(f)
	tests := []struct {
		name, text, base string
		known, removal   bool
		want             string
	}{
		{"absolute", "/p/jitso/builds/x.yaml", "/p", true, false, areaBuilds},
		{"relative", "jitso/builds/x.yaml", "/p", true, false, areaBuilds},
		{"dotdot", "docs/../jitso/jitso.yaml", "/p", true, false, areaYAML},
		{"home", "~/jitso/builds/x", "/p", true, false, areaBuilds},
		{"home, harmless", "~/notes/x", "/p", true, false, ""},
		{"home as a variable", "$HOME/jitso/builds/x", "/p", true, false, areaBuilds},
		{"project dir", "$CLAUDE_PROJECT_DIR/jitso/builds/x", "/q", true, false, areaBuilds},
		{"project dir, harmless", "$CLAUDE_PROJECT_DIR/src/x", "/q", true, false, ""},
		{"project dir braces", "${CLAUDE_PROJECT_DIR}/jitso/hard-rules", "/q", true, false, areaRules},
		{"pwd", "$PWD/jitso/builds", "/p", true, false, areaBuilds},
		{"pwd of an unknown base", "$PWD/jitso/builds", "", false, false, areaBuilds},
		{"another variable falls back to the text", "$X/jitso/builds/x", "/p", true, false, areaBuilds},
		{"a variable that hides the words", "$X/b", "/p", true, false, ""},
		{"glob falls back to the text", "jitso/builds/*", "/p", true, false, areaBuilds},
		{"brace falls back", "{a,b}/jitso/jitso.yaml", "/p", true, false, areaYAML},
		{"unknown base, relative", "jitso/builds/x", "", false, false, areaBuilds},
		{"unknown base, harmless", "src/x", "", false, false, ""},
		{"unknown base, absolute is resolved", "/z/x", "", false, false, ""},
		{"folder link", "/p/notes/b/x.yaml", "/p", true, false, areaBuilds},
		{"file link", "/p/notes/rules.md", "/p", true, false, areaRules},
		{"dangling file link", "/p/notes/new.yaml", "/p", true, false, areaBuilds},
		{"relative link", "/p/notes/rel", "/p", true, false, areaYAML},
		{"a chain of links", "/p/notes/chain/x", "/p", true, false, areaBuilds},
		{"a loop passes", "/p/notes/loop1", "/p", true, false, ""},
		{"a backslash in the word", `jit\so/builds/x`, "/p", true, false, areaBuilds},
		{"other folder", "/p/src/x.ts", "/p", true, false, ""},
		{"drafts", "/p/.jitso/drafts/x.yaml", "/p", true, false, ""},
		{"the anchor is not a write target", "/p/jitso", "/p", true, false, ""},
		{"the anchor is a removal", "/p/jitso", "/p", true, true, areaRoot},
		{"a relative anchor", "jitso", "/p", true, true, areaRoot},
		{"the dot", ".", "/p", true, true, areaRoot},
		{"the parent folder", "..", "/p", true, true, areaRoot},
		{"the primary's anchor", "/main/jitso", "/z", true, true, areaRoot},
		{"a folder above the primary", "/", "/z", true, true, areaRoot},
		{"another folder", "/tmp/x", "/p", true, true, ""},
		{"a sibling of the anchor", "/p/jitso-notes", "/p", true, true, ""},
		{"a folder inside it", "/p/jitso/dispatcher", "/p", true, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := b.PathArea(tt.text, tt.base, tt.known, tt.removal); got != tt.want {
				t.Errorf("PathArea(%q, %q) = %q, want %q", tt.text, tt.base, got, tt.want)
			}
		})
	}
	// no HOME: a ~ path cannot be expanded and gets the text check
	b.Home = ""
	if got := b.PathArea("~/jitso/builds/x", "/p", true, false); got != areaBuilds {
		t.Errorf("without HOME the text check applies, got %q", got)
	}
}

func TestToolDenial(t *testing.T) {
	b := newBoundary(fakeFS{dirs: map[string]bool{"/p": true}})
	call := func(tool string, in map[string]any, cwd any) map[string]any {
		m := map[string]any{"tool_name": tool, "tool_input": in}
		if cwd != nil {
			m["cwd"] = cwd
		}
		return m
	}
	tests := []struct {
		name  string
		input map[string]any
		want  Denial
	}{
		{"write", call("Write", map[string]any{"file_path": "/p/jitso/builds/x.yaml"}, "/p"), Denial{Verb: "Write", Area: areaBuilds}},
		{"edit", call("Edit", map[string]any{"file_path": "/p/jitso/jitso.yaml"}, "/p"), Denial{Verb: "Edit", Area: areaYAML}},
		{"multiedit", call("MultiEdit", map[string]any{"file_path": "/p/jitso/hard-rules/core.md"}, "/p"), Denial{Verb: "MultiEdit", Area: areaRules}},
		{"notebook", call("NotebookEdit", map[string]any{"notebook_path": "/p/jitso/builds/x.ipynb"}, "/p"), Denial{Verb: "NotebookEdit", Area: areaBuilds}},
		{"notebook with the wrong field", call("NotebookEdit", map[string]any{"file_path": "/p/jitso/builds/x.ipynb"}, "/p"), Denial{}},
		{"relative to the cwd", call("Write", map[string]any{"file_path": "builds/x.yaml"}, "/p/jitso"), Denial{Verb: "Write", Area: areaBuilds}},
		{"relative without a cwd uses the checkout", call("Write", map[string]any{"file_path": "jitso/builds/x.yaml"}, nil), Denial{Verb: "Write", Area: areaBuilds}},
		{"a cwd that is not a string", call("Write", map[string]any{"file_path": "jitso/builds/x.yaml"}, 5.0), Denial{Verb: "Write", Area: areaBuilds}},
		{"elsewhere", call("Write", map[string]any{"file_path": "/p/src/x.ts"}, "/p"), Denial{}},
		{"read", call("Read", map[string]any{"file_path": "/p/jitso/jitso.yaml"}, "/p"), Denial{}},
		{"missing field", call("Write", map[string]any{}, "/p"), Denial{}},
		{"wrong type", call("Write", map[string]any{"file_path": 5.0}, "/p"), Denial{}},
		{"empty path", call("Write", map[string]any{"file_path": ""}, "/p"), Denial{}},
		{"no tool_input", map[string]any{"tool_name": "Write"}, Denial{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := b.ToolDenial(tt.input); got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
	// the same call through the guard
	l := Launcher{Bounds: b}
	d := l.Guard(call("Write", map[string]any{"file_path": "/p/jitso/builds/x.yaml"}, "/p"))
	want := "JITSO: only the owner changes approved builds, jitso/jitso.yaml and the hard rules; draft a build with jitso build draft and ask the owner to save it (denied: Write jitso/builds/)"
	if got := d.Reason(nil); got != want {
		t.Errorf("reason %q, want %q", got, want)
	}
	if got := d.Reason([]byte(`{"scripts":{"jitso":"x"}}`)); !strings.Contains(got, "draft a build with bun run jitso build draft") {
		t.Errorf("reason %q", got)
	}
	if got := (Launcher{}).Guard(call("Write", map[string]any{"file_path": "/p/jitso/builds/x.yaml"}, "/p")); got.Verb != "" {
		t.Errorf("without a boundary writes are not judged, got %+v", got)
	}
}

func TestBashWrites(t *testing.T) {
	f := fakeFS{
		links: map[string]string{"/p/notes/b": "/p/jitso/builds"},
		dirs:  map[string]bool{"/p": true, "/p/jitso": true, "/p/jitso/builds": true, "/p/notes": true},
	}
	l := Launcher{Bounds: newBoundary(f)}
	tests := []struct {
		command string
		want    string // "<verb> <area>", "" when allowed
	}{
		{"cp /tmp/x jitso/builds/a.yaml", "cp jitso/builds/"},
		{"cp -t jitso/builds /tmp/x", "cp jitso/builds/"},
		{"cp --target-directory=jitso/builds /tmp/x", "cp jitso/builds/"},
		{"cp -r /tmp/x /p/jitso/hard-rules/", "cp jitso/hard-rules/"},
		{"cp -s jitso/builds/a.yaml /tmp/x", "cp jitso/builds/"},
		{"cp -l jitso/jitso.yaml /tmp/x", "cp jitso/jitso.yaml"},
		{"cp --symbolic-link jitso/builds/a.yaml /tmp/x", "cp jitso/builds/"},
		{"cp jitso/builds/haiku-explorer.yaml /tmp/x", ""},
		{"install -m 644 /tmp/x /p/jitso/builds/x.yaml", "install jitso/builds/"},
		{"rsync -a /tmp/x/ jitso/builds/", "rsync jitso/builds/"},
		{"mv /tmp/x jitso/builds/", "mv jitso/builds/"},
		{"mv jitso/jitso.yaml /tmp/x", "mv jitso/jitso.yaml"},
		{"mv -t /tmp jitso/jitso.yaml", "mv jitso/jitso.yaml"},
		{"mv jitso /tmp/x", "mv jitso/"},
		{"mv /tmp/x /tmp/y", ""},
		{"rm jitso/builds/haiku-explorer.yaml", "rm jitso/builds/"},
		{"rm -rf jitso/hard-rules", "rm jitso/hard-rules/"},
		{"rm -rf jitso", "rm jitso/"},
		{"rm -rf ./jitso/", "rm jitso/"},
		{"rm -rf .", "rm jitso/"},
		{"rm -rf /p/src", ""},
		{"rm -rf node_modules dist", ""},
		{"rmdir jitso", "rmdir jitso/"},
		{"unlink jitso/jitso.yaml", "unlink jitso/jitso.yaml"},
		{"touch jitso/builds/x.yaml", "touch jitso/builds/"},
		{"truncate -s 0 jitso/jitso.yaml", "truncate jitso/jitso.yaml"},
		{"shred -u jitso/hard-rules/core.md", "shred jitso/hard-rules/"},
		{"echo x > jitso/jitso.yaml", "redirect jitso/jitso.yaml"},
		{"echo x >jitso/jitso.yaml", "redirect jitso/jitso.yaml"},
		{"echo x>>jitso/hard-rules/core.md", "redirect jitso/hard-rules/"},
		{"echo x 2> jitso/builds/err", "redirect jitso/builds/"},
		{"echo x >| jitso/jitso.yaml", "redirect jitso/jitso.yaml"},
		{"echo x &> jitso/jitso.yaml", "redirect jitso/jitso.yaml"},
		{"echo x <> jitso/jitso.yaml", "redirect jitso/jitso.yaml"},
		{"cat > jitso/builds/x.yaml <<EOF\nid: x\nEOF", "redirect jitso/builds/"},
		{"echo jitso/builds > /tmp/list", ""},
		{"echo x > /tmp/x 2>&1", ""},
		{"cat <<EOF\nx\nEOF", ""},
		{"tee -a jitso/builds/x.yaml < /tmp/x", "tee jitso/builds/"},
		{"echo x | tee jitso/jitso.yaml", "tee jitso/jitso.yaml"},
		{"sed -i s/a/b/ jitso/builds/haiku-explorer.yaml", "sed -i jitso/builds/"},
		{"sed -i.bak s/a/b/ jitso/jitso.yaml", "sed -i jitso/jitso.yaml"},
		{"sed --in-place s/a/b/ jitso/jitso.yaml", "sed -i jitso/jitso.yaml"},
		{"sed -ni s/a/b/p jitso/jitso.yaml", "sed -i jitso/jitso.yaml"},
		{"sed -i -e s/a/b/ jitso/jitso.yaml", "sed -i jitso/jitso.yaml"},
		{"sed -n 1p jitso/jitso.yaml", ""},
		{"sed s/a/b/ jitso/jitso.yaml", ""},
		{"sed -e s/i/x/ jitso/jitso.yaml", ""},
		{"perl -pi -e s/a/b/ jitso/jitso.yaml", "perl -i jitso/jitso.yaml"},
		{"perl -i.bak -e s/a/b/ jitso/builds/a.yaml", "perl -i jitso/builds/"},
		{"perl -ne print jitso/jitso.yaml", ""},
		{"yq -i .x=1 jitso/builds/a.yaml", "yq -i jitso/builds/"},
		{"yq .x jitso/builds/a.yaml", ""},
		{"dd if=/tmp/x of=jitso/jitso.yaml", "dd jitso/jitso.yaml"},
		{"dd if=jitso/jitso.yaml of=/tmp/x", ""},
		{"ln -s /tmp/x jitso/builds/x.yaml", "ln jitso/builds/"},
		{"ln jitso/jitso.yaml /tmp/j", "ln jitso/jitso.yaml"},
		{"git mv jitso/builds/a.yaml /tmp/a.yaml", "git mv jitso/builds/"},
		{"git mv jitso /tmp/j", "git mv jitso/"},
		{"git rm -r jitso", "git rm jitso/"},
		{"git rm src/x.ts", ""},
		{"git checkout HEAD -- jitso/hard-rules", "git checkout jitso/hard-rules/"},
		{"git -C /p restore jitso/jitso.yaml", "git restore jitso/jitso.yaml"},
		{"git checkout main", ""},
		{"git diff jitso/jitso.yaml", ""},
		{"git add jitso/builds/haiku-explorer.yaml", ""},
		{"git commit -m jitso/builds", ""},
		{"cat jitso/builds/haiku-explorer.yaml", ""},
		{"grep -r id jitso/builds", ""},
		{"ls jitso/hard-rules", ""},
		{"diff jitso/jitso.yaml /tmp/j", ""},
		{"cd jitso && mv /tmp/x builds/", "mv jitso/builds/"},
		{"cd jitso/builds && cp /tmp/x .", "cp jitso/builds/"},
		{"cd jitso; rm -rf builds", "rm jitso/builds/"},
		{"pushd jitso/builds && touch x", "touch jitso/builds/"},
		{"cd /tmp && cp x jitso/builds/", "cp jitso/builds/"}, // any folder named jitso is a catalog
		{"cd /tmp && cp x src/", ""},
		{"cd /p/jitso && cp x builds/", "cp jitso/builds/"},
		{"cd $X && cp x jitso/builds/", "cp jitso/builds/"},
		{"cd && cp x /tmp/y", ""},
		{"cd - && cp x builds/", ""}, // open: the folder is unknown and the word holds no jitso/
		{"cd - && cp x jitso/builds/", "cp jitso/builds/"},
		{"cd jitso && cd .. && cp x src/", ""},
		{"sh -c 'cp /tmp/x jitso/builds/'", "cp jitso/builds/"},
		{"bash -lc \"echo x > jitso/jitso.yaml\"", "redirect jitso/jitso.yaml"},
		{"sh -c 'cd jitso && cp x builds/'", "cp jitso/builds/"},
		{"sudo cp /tmp/x jitso/builds/", "cp jitso/builds/"},
		{"env FOO=1 mv /tmp/x jitso/builds/", "mv jitso/builds/"},
		{"cp /tmp/x $CLAUDE_PROJECT_DIR/jitso/builds/", "cp jitso/builds/"},
		{"cp /tmp/x ~/jitso/builds/", "cp jitso/builds/"},
		{"cp /tmp/x notes/b/x.yaml", "cp jitso/builds/"},
		{"cp /tmp/x $X/builds", ""},
		{"cp /tmp/x jitso/b*", ""},
		{"cp /tmp/x jitso/builds/*", "cp jitso/builds/"},
		{"echo hi", ""},
		{"ls && pwd", ""},
		{"git status", ""},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			d := l.bashDenial(tt.command, "/p")
			got := ""
			if d.Verb != "" {
				got = d.Verb + " " + d.Area
			}
			if got != tt.want {
				t.Errorf("%q: got %q, want %q", tt.command, got, tt.want)
			}
		})
	}
}

// A later check never hides an earlier one: the denial of the first segment
// wins, and within one segment the order is build save, adoption, launch verbs,
// a harness, then a write.
func TestWriteDenialOrder(t *testing.T) {
	l := Launcher{Bounds: newBoundary(fakeFS{dirs: map[string]bool{"/p": true}})}
	for command, want := range map[string]string{
		"cp x jitso/builds/ && claude":                          "cp",
		"claude && cp x jitso/builds/":                          "claude",
		"jitso build save x && cp x jitso/builds/":              VerbBuildSave,
		"echo x > jitso/jitso.yaml; jitso setup hook":           "redirect",
		"orca terminal create --command claude && rm -rf jitso": "terminal create --command <harness>",
	} {
		if got := l.bashDenial(command, "/p").Verb; got != want {
			t.Errorf("%q: got %q, want %q", command, got, want)
		}
	}
}

func TestRedirectTargets(t *testing.T) {
	tests := []struct {
		words   string
		targets string
		pending bool
	}{
		{"echo x > a", "a", false},
		{"echo x >a", "a", false},
		{"echo x>>a", "a", false},
		{"echo x 2>a", "a", false},
		{"echo x 2> a", "a", false},
		{"echo x >", "", true},
		{"echo x <> a", "a", false},
		{"cat <<EOF", "", false},
		{"cat < a", "", false},
		{"echo x > a > b", "a b", false},
		{"echo a>b>c", "b c", false},
	}
	for _, tt := range tests {
		got, pending := redirectTargets(strings.Split(tt.words, " "))
		if strings.Join(got, " ") != tt.targets || pending != tt.pending {
			t.Errorf("%q: targets %q pending %v, want %q %v", tt.words, got, pending, tt.targets, tt.pending)
		}
	}
}
