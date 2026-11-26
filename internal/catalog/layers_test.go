package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

// tree makes real folders for the layer tests, all under one temp folder whose
// real path is used (on macOS the temp folder is behind a symlink).
type tree struct {
	t    *testing.T
	root string
}

func newTree(t *testing.T) *tree {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &tree{t: t, root: root}
}

func (tr *tree) path(rel string) string { return filepath.Join(tr.root, filepath.FromSlash(rel)) }

func (tr *tree) mkdir(rel string) string {
	tr.t.Helper()
	if err := os.MkdirAll(tr.path(rel), 0o755); err != nil {
		tr.t.Fatal(err)
	}
	return tr.path(rel)
}

func (tr *tree) write(rel, text string) {
	tr.t.Helper()
	tr.mkdir(filepath.ToSlash(filepath.Dir(filepath.FromSlash(rel))))
	if err := os.WriteFile(tr.path(rel), []byte(text), 0o644); err != nil {
		tr.t.Fatal(err)
	}
}

const layerAllowlists = "version: 1\nmodels: [claude-haiku-5-5]\nefforts: [low]\nmcp: []\nskills: []\ntools: [Read]\n"

// project writes the allowlists and the one instruction of a project at rel.
func (tr *tree) project(rel string) string {
	tr.t.Helper()
	tr.write(rel+"/jitso/jitso.yaml", layerAllowlists)
	tr.write(rel+"/.agents/instructions/agent-project.md", "---\nid: project\n---\n")
	return tr.path(rel)
}

// layerBuild is a valid worker build with the given id and description.
func layerBuild(id, desc string, extra ...string) string {
	s := "id: " + id + "\nversion: 1\nstatus: approved\nmodel: claude-haiku-5-5\neffort: low\ndescription: " + desc +
		"\nroute_scope: [none]\ntools: [Read]\n"
	return s + strings.Join(extra, "\n") + map[bool]string{true: "\n", false: ""}[len(extra) > 0]
}

func layerSubagent(id, prompt string) string {
	return "id: " + id + "\nversion: 1\nstatus: approved\ntier: subagent\nmodel: claude-haiku-5-5\neffort: low\ndescription: helper\nprompt: " + prompt + "\n"
}

func TestSources(t *testing.T) {
	primary := t.TempDir()
	got := Sources(primary)
	want := []Source{
		{Layer: LayerRepo, Dir: filepath.Join(primary, "jitso", "builds"), Display: "jitso/builds"},
		{Layer: LayerDraft, Dir: filepath.Join(primary, ".jitso", "drafts"), Display: ".jitso/drafts"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sources\n got %+v\nwant %+v", got, want)
	}
}

// folder is a source over an in-memory folder (the files are at its root).
func folder(layer Layer, display string, files map[string]string) Source {
	fsys := fstest.MapFS{}
	for name, body := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return Source{Layer: layer, Dir: "/" + display, Display: display, fsys: fsys}
}

var layerConfig = Config{Models: []string{"claude-haiku-5-5"}, Efforts: []string{"low"}, Tools: []string{"Read"}}

func scan(t *testing.T, sources ...Source) *Result {
	t.Helper()
	res, err := ScanSources(layerConfig, []string{"project"}, sources)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func ids(files []*File) []string {
	var out []string
	for _, f := range files {
		out = append(out, string(f.Layer)+":"+f.Name)
	}
	return out
}

// Inside one layer the first file by name is the build of an id, and the
// second is shadowed.
func TestFirstFileWinsInsideALayer(t *testing.T) {
	dir := folder(LayerRepo, "dir", map[string]string{
		"z-dup.yaml": layerBuild("dup", "SECOND"),
		"a-dup.yaml": layerBuild("dup", "FIRST"),
	})
	res := scan(t, dir)
	cat, err := res.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Builds) != 1 || cat.Builds[0].File != "a-dup.yaml" {
		t.Errorf("builds %+v", cat.Builds)
	}
	if second := res.Files[len(res.Files)-1]; second.Name != "z-dup.yaml" || second.ShadowedBy == nil || second.ShadowedBy.Name != "a-dup.yaml" {
		t.Errorf("second file %+v", second)
	}
}

// A saved file shadows a draft of its id, and a draft of a new id is a build
// of the draft layer.
func TestScanShadowing(t *testing.T) {
	repo := folder(LayerRepo, "jitso/builds", map[string]string{
		"a.yaml": layerBuild("alpha", "repo alpha"),
		"b.yaml": layerBuild("beta", "repo beta"),
	})
	drafts := folder(LayerDraft, ".jitso/drafts", map[string]string{
		"alpha.yaml": draftBuild("alpha", "draft alpha"),
		"gamma.yaml": draftBuild("gamma", "draft gamma"),
	})
	res := scan(t, repo, drafts)
	want := []string{"repo:a.yaml", "repo:b.yaml", "draft:alpha.yaml", "draft:gamma.yaml"}
	if got := ids(res.Files); !reflect.DeepEqual(got, want) {
		t.Fatalf("files %v", got)
	}
	shadowed := map[string]string{}
	for _, f := range res.Files {
		if f.ShadowedBy != nil {
			shadowed[f.Display] = f.ShadowedBy.Display
		}
	}
	wantShadowed := map[string]string{".jitso/drafts/alpha.yaml": "jitso/builds/a.yaml"}
	if !reflect.DeepEqual(shadowed, wantShadowed) {
		t.Errorf("shadowed %v\nwant     %v", shadowed, wantShadowed)
	}

	cat, err := res.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, b := range cat.Builds {
		got = append(got, b.ID+"/"+string(b.Layer)+"/"+b.Description)
	}
	wantBuilds := []string{"alpha/repo/repo alpha", "beta/repo/repo beta", "gamma/draft/draft gamma"}
	if !reflect.DeepEqual(got, wantBuilds) {
		t.Errorf("builds %v\nwant   %v", got, wantBuilds)
	}
	if got, want := cat.List(), "- alpha: claude-haiku-5-5 low. repo alpha requires: nothing\n- beta: claude-haiku-5-5 low. repo beta requires: nothing"; got != want {
		t.Errorf("list:\n%s", got)
	}
}

// Draft files that are invalid here are skipped, not fatal.
func TestDraftFilesInvalidHereAreSkipped(t *testing.T) {
	drafts := folder(LayerDraft, ".jitso/drafts", map[string]string{
		"a-broken.yaml":     "id: [",
		"b-noid.yaml":       "version: 1\n",
		"c-bad-id.yaml":     strings.Replace(draftBuild("x", "d"), "id: x", "id: Not Kebab", 1),
		"d-route.yaml":      strings.Replace(draftBuild("route", "d"), "route_scope: [none]", "route_scope: [elsewhere]", 1),
		"e-missing.yaml":    draftBuild("lost", "d", "subagents: [nobody]"),
		"f-worker.yaml":     draftBuild("worker-sub", "d", "subagents: [worker-tier]"),
		"g-workertier.yaml": draftBuild("worker-tier", "d"),
		"h-fine.yaml":       draftBuild("fine", "d"),
		"i-notyaml.txt":     "ignored",
	})
	cat, err := scan(t, folder(LayerRepo, "jitso/builds", nil), drafts).Catalog()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, s := range cat.Skipped {
		got[s.Display] = s.ID + "|" + strings.Join(s.Problems, "; ")
	}
	for display, want := range map[string]string{
		".jitso/drafts/b-noid.yaml":    "|id must be kebab-case",
		".jitso/drafts/c-bad-id.yaml":  "|id must be kebab-case",
		".jitso/drafts/d-route.yaml":   `route|route_scope id "elsewhere" is not an instruction section`,
		".jitso/drafts/e-missing.yaml": `lost|subagent "nobody" is not a subagent-tier build`,
		".jitso/drafts/f-worker.yaml":  `worker-sub|subagent "worker-tier" is not a subagent-tier build`,
	} {
		if display == ".jitso/drafts/b-noid.yaml" {
			if !strings.HasPrefix(got[display], want) {
				t.Errorf("%s: got %q, want it to start with %q", display, got[display], want)
			}
			continue
		}
		if got[display] != want {
			t.Errorf("%s: got %q, want %q", display, got[display], want)
		}
	}
	if !strings.HasPrefix(got[".jitso/drafts/a-broken.yaml"], "|") || len(got) != 6 {
		t.Errorf("skipped %v", got)
	}
	if _, found := cat.Find("fine"); !found {
		t.Error("the valid file must stay")
	}
	if _, found := cat.Find("worker-tier"); !found {
		t.Error("a valid worker-tier build stays a build")
	}
}

// The project's own files keep the whole-catalog rule: the first invalid file
// in file name order is the error, and a subagent that does not resolve comes
// after every file error.
func TestProjectFilesStayStrict(t *testing.T) {
	bad := strings.Replace(layerBuild("broken", "d"), "claude-haiku-5-5", "gpt", 1)
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"invalid file", map[string]string{"z.yaml": bad}, `build jitso/builds/z.yaml is invalid: model "gpt" is not in the allowlist`},
		{"file errors come first", map[string]string{"a.yaml": layerBuild("lost", "d", "subagents: [nobody]"), "b.yaml": bad},
			`build jitso/builds/b.yaml is invalid: model "gpt" is not in the allowlist`},
		{"subagent", map[string]string{"a.yaml": layerBuild("lost", "d", "subagents: [nobody]")}, `build jitso/builds/a.yaml is invalid: subagent "nobody" is not a subagent-tier build`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := scan(t, folder(LayerRepo, "jitso/builds", tt.files)).Catalog()
			var fe *FileError
			if !errors.As(err, &fe) || err.Error() != tt.want {
				t.Errorf("got %v, want %s", err, tt.want)
			}
		})
	}
}

// A worker resolves its subagents with the same lookup as every build.
func TestSubagentsResolveThroughTheLayers(t *testing.T) {
	repo := folder(LayerRepo, "jitso/builds", map[string]string{
		"plan.yaml":   layerSubagent("plan", "REPO PLAN"),
		"helper.yaml": layerSubagent("helper", "REPO HELPER"),
		"team.yaml":   layerBuild("team", "d", "subagents: [plan, helper]"),
	})
	cat, err := scan(t, repo).Catalog()
	if err != nil {
		t.Fatal(err)
	}
	team, _ := cat.Find("team")
	subs := cat.SubagentsOf(team)
	if len(subs) != 2 || subs[0].Prompt != "REPO PLAN" || subs[0].Layer != LayerRepo || subs[1].Prompt != "REPO HELPER" || subs[1].Layer != LayerRepo {
		t.Errorf("subagents %+v", subs)
	}
}

// LoadLayers over real folders: the layers come from the primary's real path.
// A folder of builds under HOME is never read.
func TestLoadLayers(t *testing.T) {
	tr := newTree(t)
	primary := tr.project("repo")
	tr.write("repo/jitso/builds/mine.yaml", layerBuild("mine", "repo"))
	tr.write("repo/.jitso/drafts/scratch.yaml", draftBuild("scratch", "draft"))
	tr.write("home/.jitso/builds/user-only.yaml", layerBuild("user-only", "user"))
	tr.write("home/.jitso/config.yaml", "trusted_parents:\n  - "+tr.path("repo")+"\n")

	cat, err := LoadLayers(os.DirFS(primary), primary)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, b := range cat.Builds {
		got = append(got, string(b.Layer)+":"+b.ID+":"+b.Display)
	}
	want := []string{"repo:mine:jitso/builds/mine.yaml", "draft:scratch:.jitso/drafts/scratch.yaml"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("builds\n got %v\nwant %v", got, want)
	}
	mine, _ := cat.Find("mine")
	data, _ := os.ReadFile(tr.path("repo/jitso/builds/mine.yaml"))
	if mine.Path != tr.path("repo/jitso/builds/mine.yaml") || mine.SHA256 != sha256Hex(data) {
		t.Errorf("mine: path %q sha %q", mine.Path, mine.SHA256)
	}
}

func TestLoadLayersMissingPieces(t *testing.T) {
	tr := newTree(t)
	primary := tr.mkdir("repo")
	if cat, err := LoadLayers(os.DirFS(primary), primary); err != nil || len(cat.Builds) != 0 {
		t.Errorf("no jitso.yaml is an empty project with open lists: %v %+v", err, cat)
	}
	tr.project("repo")
	cat, err := LoadLayers(os.DirFS(primary), primary)
	if err != nil || len(cat.Builds) != 0 {
		t.Errorf("no jitso/builds folder is an empty project layer: %v %+v", err, cat)
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Tolerant never fails: an invalid file of the project is skipped and has no
// Build, where Load fails on it; a valid file has a Build with its origin.
func TestTolerantAndBuild(t *testing.T) {
	broken := strings.Replace(validBuild, "id: haiku-explorer", "id: broken", 1)
	broken = strings.Replace(broken, "model: claude-haiku-5-5", "model: nope", 1)
	fsys := testFS(map[string]string{"a-ok.yaml": validBuild, "b-broken.yaml": broken})
	if _, err := Load(fsys); err == nil {
		t.Fatal("Load must fail on the invalid repo file")
	}
	res, err := Scan(fsys, "")
	if err != nil {
		t.Fatal(err)
	}
	cat := res.Tolerant()
	if len(cat.Builds) != 1 || cat.Builds[0].ID != "haiku-explorer" || cat.Builds[0].Display != "jitso/builds/a-ok.yaml" {
		t.Errorf("builds: %+v", cat.Builds)
	}
	if len(cat.Skipped) != 1 || cat.Skipped[0].ID != "broken" {
		t.Errorf("skipped: %+v", cat.Skipped)
	}
	if f := res.Winner("broken"); f == nil || f.Build() != nil {
		t.Errorf("an invalid file has no Build: %+v", f)
	}
	b := res.Winner("haiku-explorer").Build()
	if b == nil || b.File != "a-ok.yaml" || b.Layer != LayerRepo || b.SHA256 == "" {
		t.Errorf("Build: %+v", b)
	}
}
