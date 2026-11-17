package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/fail"
)

// projectWith writes a project on disk (a catalog needs the draft folder to be
// a real one): the allowlists, the saved builds and the drafts. It returns the
// catalog of its layers, the draft layer last.
func projectWith(t *testing.T, saved, drafts map[string]string) *catalog.Catalog {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	put := func(rel, text string) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put("jitso/jitso.yaml", config)
	put(".agents/instructions/agent-ticket-work.md", "---\nid: ticket-work\n---\n")
	for name, text := range saved {
		put("jitso/builds/"+name, text)
	}
	for name, text := range drafts {
		put(".jitso/drafts/"+name, text)
	}
	res, err := catalog.Scan(os.DirFS(root), root)
	if err != nil {
		t.Fatal(err)
	}
	cat, err := res.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func TestResolveDrafts(t *testing.T) {
	helper := func(id, status string) string {
		return build(id, "tier: subagent", "prompt: You help.", "status: "+status)
	}
	cat := projectWith(t,
		map[string]string{
			"saved.yaml":      build("saved"),
			"cand-help.yaml":  helper("cand-help", "candidate"),
			"uses-draft.yaml": build("uses-draft", "subagents: [scratch]"),
			"old.yaml":        build("old", "status: candidate"),
		},
		map[string]string{
			"scratch.yaml":     helper("scratch", "draft"),
			"scout.yaml":       build("scout", "status: draft"),
			"team.yaml":        build("team", "status: draft", "subagents: [scratch]"),
			"picky.yaml":       build("picky", "status: draft", "subagents: [cand-help]"),
			"saved-sub.yaml":   build("saved-sub", "status: draft", "subagents: [scratch, cand-help]"),
			"helper-only.yaml": helper("helper-only", "draft"),
			"old-draft.yaml":   build("old", "status: draft"),
		})

	tests := []struct {
		name   string
		build  string
		reason string
	}{
		{"a draft launches", "scout", ""},
		{"a saved build launches", "saved", ""},
		{"a draft worker may list a draft subagent", "team", ""},
		{"a draft worker may not list a candidate subagent", "picky", `build "picky" lists subagent "cand-help", which is candidate, not approved`},
		{"a draft worker names the first subagent it may not use", "saved-sub", `build "saved-sub" lists subagent "cand-help", which is candidate, not approved`},
		{"a saved worker may not list a draft subagent", "uses-draft", `build "uses-draft" lists subagent "scratch", which is draft, not approved`},
		{"a draft subagent is not launched alone", "helper-only", `build "helper-only" has tier subagent: it is not launched alone, a worker lists it in its subagents`},
		{"a saved candidate still does not launch, a draft of its id does not replace it", "old", `build "old" is candidate, not approved`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Resolve(Request{Build: tt.build}, cat)
			if tt.reason == "" {
				if err != nil {
					t.Fatalf("rejected: %v", err)
				}
				return
			}
			if err == nil || fail.Code(err) != fail.CodeRejected || fail.Text(err) != "JITSO REJECTED: "+tt.reason {
				t.Errorf("error %v, want %q", err, tt.reason)
			}
		})
	}

	p, err := Resolve(Request{Build: "scout"}, cat)
	if err != nil || p.Build.Layer != catalog.LayerDraft || p.Build.Status != catalog.StatusDraft {
		t.Fatalf("the profile of a draft: %+v, %v", p.Build, err)
	}
}

// LaunchProblems (what `build validate` prints) follows the same rule.
func TestLaunchProblemsDrafts(t *testing.T) {
	cat := projectWith(t,
		map[string]string{"uses-draft.yaml": build("uses-draft", "subagents: [scratch]")},
		map[string]string{
			"scratch.yaml": build("scratch", "tier: subagent", "prompt: You help.", "status: draft"),
			"team.yaml":    build("team", "status: draft", "subagents: [scratch]"),
		})
	for id, want := range map[string][]string{
		"uses-draft": {`subagent "scratch" is draft, not approved`},
		"team":       nil,
	} {
		b, ok := cat.Find(id)
		if !ok {
			t.Fatalf("no build %s", id)
		}
		got := LaunchProblems(b, cat)
		if len(got) != len(want) || (len(got) == 1 && got[0] != want[0]) {
			t.Errorf("%s: problems %q, want %q", id, got, want)
		}
	}
}
