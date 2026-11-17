package catalog

import (
	"reflect"
	"strings"
	"testing"
)

// draftBuild is a valid worker draft of the layer tests: layerBuild with the
// status of a draft.
func draftBuild(id, desc string, extra ...string) string {
	return strings.Replace(layerBuild(id, desc, extra...), "status: approved", "status: draft", 1)
}

func draftSubagent(id string) string {
	return strings.Replace(layerSubagent(id, "Read a folder."), "status: approved", "status: draft", 1)
}

// The status rule is the one thing the layer changes: a saved layer takes
// candidate, approved or retired, the draft layer takes exactly draft.
func TestValidateBuildInStatusByLayer(t *testing.T) {
	const savedProblem = "status must be one of candidate, approved, retired"
	const draftProblem = "status must be draft in a draft"
	tests := []struct {
		name   string
		status string // the edit; "status:-" removes the key
		layer  Layer
		want   []string
	}{
		{"draft in the draft layer", "status: draft", LayerDraft, nil},
		{"approved in the draft layer", "status: approved", LayerDraft, []string{draftProblem}},
		{"candidate in the draft layer", "status: candidate", LayerDraft, []string{draftProblem}},
		{"retired in the draft layer", "status: retired", LayerDraft, []string{draftProblem}},
		{"missing in the draft layer", "status:-", LayerDraft, []string{draftProblem}},
		{"a number in the draft layer", "status: 1", LayerDraft, []string{draftProblem}},
		{"Draft in the draft layer", "status: Draft", LayerDraft, []string{draftProblem}},
		{"draft in the repo layer", "status: draft", LayerRepo, []string{savedProblem}},
		{"draft in a parent layer", "status: draft", LayerParent, []string{savedProblem}},
		{"draft in the user layer", "status: draft", LayerUser, []string{savedProblem}},
		{"approved in the repo layer", "status: approved", LayerRepo, nil},
		{"candidate in the user layer", "status: candidate", LayerUser, nil},
		{"missing in the repo layer", "status:-", LayerRepo, []string{savedProblem}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateBuildIn(with(t, tt.status), testConfig, testRouteIDs, tt.layer)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("problems %q, want %q", got, tt.want)
			}
		})
	}
	// ValidateBuild is the saved-layer rule.
	if got := ValidateBuild(with(t, "status: draft"), testConfig, testRouteIDs); !reflect.DeepEqual(got, []string{savedProblem}) {
		t.Errorf("ValidateBuild of a draft = %q", got)
	}
}

func draftScan(t *testing.T, repo, drafts map[string]string, outer ...Source) *Result {
	t.Helper()
	sources := []Source{folder(LayerRepo, "jitso/builds", repo)}
	sources = append(sources, outer...)
	sources = append(sources, folder(LayerDraft, ".jitso/drafts", drafts))
	return scan(t, sources...)
}

// The draft layer is last, a draft launches as a build of its layer, and a
// draft is never a launchable build: not listed.
func TestDraftLayerIsLastAndNeverLaunchable(t *testing.T) {
	res := draftScan(t,
		map[string]string{"saved.yaml": layerBuild("saved", "A saved build.")},
		map[string]string{"scratch.yaml": draftBuild("scratch", "A draft build.")},
		folder(LayerUser, "~/.jitso/builds", map[string]string{"mine.yaml": layerBuild("mine", "A user build.")}))
	if got, want := ids(res.Files), []string{"repo:saved.yaml", "user:mine.yaml", "draft:scratch.yaml"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("files %q, want %q", got, want)
	}
	cat, err := res.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	b, ok := cat.Find("scratch")
	if !ok || b.Layer != LayerDraft || b.Status != StatusDraft || b.Display != ".jitso/drafts/scratch.yaml" || b.SHA256 == "" {
		t.Fatalf("the draft build: %+v (found %v)", b, ok)
	}
	if text := cat.List(); strings.Contains(text, "scratch") || !strings.Contains(text, "- saved:") {
		t.Errorf("a listing holds a draft or lost a saved build:\n%s", text)
	}
	for _, l := range cat.Launchable() {
		if l.ID == "scratch" {
			t.Errorf("a draft is launchable: %+v", l)
		}
	}
}

// A draft never shadows a saved build, whichever layer saves it and whether or
// not the saved build is valid here: the saved file is the build of the id.
func TestDraftNeverShadowsASavedBuild(t *testing.T) {
	tests := []struct {
		name  string
		saved Source
	}{
		{"repo", folder(LayerRepo, "jitso/builds", map[string]string{"dup.yaml": layerBuild("dup", "SAVED")})},
		{"user", folder(LayerUser, "~/.jitso/builds", map[string]string{"dup.yaml": layerBuild("dup", "SAVED")})},
		{"an invalid saved build", folder(LayerUser, "~/.jitso/builds", map[string]string{"dup.yaml": strings.Replace(layerBuild("dup", "SAVED"), "claude-haiku-5-5", "gpt", 1)})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sources := []Source{folder(LayerRepo, "jitso/builds", nil)}
			if tt.saved.Layer != LayerRepo {
				sources = append(sources, tt.saved)
			} else {
				sources[0] = tt.saved
			}
			sources = append(sources, folder(LayerDraft, ".jitso/drafts", map[string]string{"dup.yaml": draftBuild("dup", "DRAFT")}))
			res := scan(t, sources...)
			draft := res.Files[len(res.Files)-1]
			if draft.Layer != LayerDraft || draft.ShadowedBy == nil || draft.ShadowedBy.Layer == LayerDraft {
				t.Fatalf("the draft is not shadowed by the saved build: %+v", draft)
			}
			if w := res.Winner("dup"); w == nil || w.Layer == LayerDraft {
				t.Errorf("winner %+v", w)
			}
			if saved := res.Saved("dup"); saved == nil || saved.Layer == LayerDraft {
				t.Errorf("Saved %+v", saved)
			}
			cat, err := res.Catalog()
			if err != nil {
				t.Fatal(err)
			}
			if b, ok := cat.Find("dup"); ok && b.Layer == LayerDraft {
				t.Errorf("a draft is the build of a saved id: %+v", b)
			}
		})
	}
}

// An invalid draft never fails the catalog: it is skipped, named in Skipped
// with its layer, and the id is unusable.
func TestInvalidDraftIsSkipped(t *testing.T) {
	res := draftScan(t, map[string]string{"saved.yaml": layerBuild("saved", "A saved build.")},
		map[string]string{"bad.yaml": strings.Replace(draftBuild("bad", "x"), "claude-haiku-5-5", "gpt", 1)})
	cat, err := res.Catalog()
	if err != nil {
		t.Fatalf("an invalid draft failed the catalog: %v", err)
	}
	if _, ok := cat.Find("bad"); ok {
		t.Error("an invalid draft is a build")
	}
	s, ok := cat.Invalid("bad")
	if !ok || s.Layer != LayerDraft {
		t.Fatalf("Invalid(bad) = %+v, %v", s, ok)
	}
	want := `jitso: warning: skipped build draft .jitso/drafts/bad.yaml: model "gpt" is not in the allowlist`
	if s.Warning() != want {
		t.Errorf("warning %q, want %q", s.Warning(), want)
	}
}

// A saved file that claims to be a draft is invalid, so the project's own
// file keeps the whole-catalog rule.
func TestDraftStatusInASavedLayerIsInvalid(t *testing.T) {
	res := draftScan(t, map[string]string{"early.yaml": draftBuild("early", "x")}, nil)
	_, err := res.Catalog()
	if err == nil || !strings.Contains(err.Error(), "build jitso/builds/early.yaml is invalid: status must be one of candidate, approved, retired") {
		t.Errorf("Catalog error = %v", err)
	}
}

func TestDraftLine(t *testing.T) {
	res := draftScan(t,
		map[string]string{"saved.yaml": layerBuild("saved", "A saved build.")},
		map[string]string{
			"a-ok.yaml":      draftBuild("a-ok", "Fine."),
			"b-shadow.yaml":  draftBuild("saved", "Same id as a saved build."),
			"c-bad.yaml":     strings.Replace(draftBuild("c-bad", "Broken."), "claude-haiku-5-5", "gpt", 1),
			"d-list.yaml":    "- a\n- b\n",
			"e-garbage.yaml": "id: [unclosed\n",
			"f-noid.yaml":    "status: draft\n",
		})
	var got []string
	for _, f := range res.Drafts() {
		got = append(got, DraftLine(f))
	}
	want := []string{
		"- a-ok: claude-haiku-5-5 low. Fine. requires: nothing (draft .jitso/drafts/a-ok.yaml)",
		"- saved: claude-haiku-5-5 low. Same id as a saved build. requires: nothing (draft .jitso/drafts/b-shadow.yaml) shadowed by repo jitso/builds/saved.yaml",
		`- c-bad: gpt low. Broken. requires: nothing (draft .jitso/drafts/c-bad.yaml) invalid here: model "gpt" is not in the allowlist`,
		"- d-list: undefined undefined. undefined requires: nothing (draft .jitso/drafts/d-list.yaml) invalid here: build is not a mapping",
	}
	if len(got) != 6 || !reflect.DeepEqual(got[:4], want) {
		t.Fatalf("lines\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if !strings.HasPrefix(got[4], "- e-garbage: undefined undefined. undefined requires: nothing (draft .jitso/drafts/e-garbage.yaml) invalid here: ") {
		t.Errorf("a file that is not YAML: %q", got[4])
	}
	if !strings.HasPrefix(got[5], "- f-noid: undefined undefined. undefined requires: nothing (draft .jitso/drafts/f-noid.yaml) invalid here: id must be kebab-case; ") {
		t.Errorf("a file with no id: %q", got[5])
	}
}

func TestCandidate(t *testing.T) {
	scanOf := func() *Result {
		return draftScan(t,
			map[string]string{
				"saved.yaml":  layerBuild("saved", "A saved build."),
				"helper.yaml": layerSubagent("helper", "Saved helper."),
			},
			map[string]string{
				"old.yaml":     draftBuild("old", "Old draft."),
				"scout.yaml":   draftBuild("scout", "FIRST"),
				"scratch.yaml": draftSubagent("scratch"),
			})
	}
	t.Run("replaces its own draft", func(t *testing.T) {
		res := scanOf()
		before := ids(res.Files)
		cand, f := res.Candidate([]byte(draftBuild("scout", "SECOND")))
		if !f.Valid() || f.ID != "scout" || f.Name != "scout.yaml" || f.Display != ".jitso/drafts/scout.yaml" || f.Layer != LayerDraft {
			t.Fatalf("file %+v", f)
		}
		if got, want := ids(cand.Files), []string{"repo:helper.yaml", "repo:saved.yaml", "draft:old.yaml", "draft:scout.yaml", "draft:scratch.yaml"}; !reflect.DeepEqual(got, want) {
			t.Errorf("files %q, want %q", got, want)
		}
		if w := cand.Winner("scout"); w != f || f.ShadowedBy != nil {
			t.Errorf("the candidate does not win its id: %+v", w)
		}
		if !reflect.DeepEqual(ids(res.Files), before) || res.Winner("scout").Raw.Value("description") != "FIRST" {
			t.Error("Candidate changed the scan it was called on")
		}
	})
	t.Run("a new id", func(t *testing.T) {
		cand, f := scanOf().Candidate([]byte(draftBuild("brand-new", "New.")))
		if !f.Valid() || cand.Winner("brand-new") != f || len(cand.Files) != 6 {
			t.Fatalf("file %+v of %d files", f, len(cand.Files))
		}
	})
	t.Run("an id a saved build has is shadowed", func(t *testing.T) {
		_, f := scanOf().Candidate([]byte(draftBuild("saved", "x")))
		if f.ShadowedBy == nil || f.ShadowedBy.Layer != LayerRepo {
			t.Errorf("file %+v", f)
		}
	})
	t.Run("the draft rules apply", func(t *testing.T) {
		_, f := scanOf().Candidate([]byte(layerBuild("scout", "approved text")))
		if !reflect.DeepEqual(f.Problems, []string{"status must be draft in a draft"}) {
			t.Errorf("problems %q", f.Problems)
		}
	})
	t.Run("an input with no id has no file name", func(t *testing.T) {
		cand, f := scanOf().Candidate([]byte("- a\n"))
		if f.Name != "" || f.ID != "" || f.Valid() || len(cand.Files) != 6 {
			t.Errorf("file %+v of %d files", f, len(cand.Files))
		}
	})
	subagents := []struct {
		name string
		list string
		want []string
	}{
		{"a saved subagent", "helper", nil},
		{"a draft subagent", "scratch", nil},
		{"a name no build has", "ghost", []string{`subagent "ghost" is not a subagent-tier build`}},
		{"a worker is not a subagent", "saved", []string{`subagent "saved" is not a subagent-tier build`}},
	}
	for _, tt := range subagents {
		t.Run("subagents: "+tt.name, func(t *testing.T) {
			_, f := scanOf().Candidate([]byte(draftBuild("scout", "x", "subagents: ["+tt.list+"]")))
			if !reflect.DeepEqual(f.Problems, tt.want) {
				t.Errorf("problems %q, want %q", f.Problems, tt.want)
			}
		})
	}
	t.Run("a replaced draft stops being a subagent", func(t *testing.T) {
		res := draftScan(t, nil, map[string]string{
			"scratch.yaml": draftSubagent("scratch"),
			"team.yaml":    draftBuild("team", "x", "subagents: [scratch]"),
		})
		if !res.Winner("team").Valid() {
			t.Fatal("the starting scan is not valid")
		}
		// Replace the helper with a worker: the other draft's subagent list
		// is checked again in the candidate, and the scan itself is untouched.
		cand, _ := res.Candidate([]byte(draftBuild("scratch", "now a worker")))
		if team := cand.Winner("team"); team.Valid() {
			t.Errorf("team still valid: %+v", team)
		}
		if !res.Winner("team").Valid() {
			t.Error("Candidate changed the receiver")
		}
	})
}
