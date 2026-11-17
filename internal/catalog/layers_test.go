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

	"github.com/upex-galaxy/jitso/internal/fail"
)

// tree makes real folders for the layer tests: a HOME and a project nested in
// two parent folders, all under one temp folder whose real path is used (on
// macOS the temp folder is behind a symlink).
type tree struct {
	t    *testing.T
	root string
	home string
}

func newTree(t *testing.T) *tree {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tr := &tree{t: t, root: root, home: filepath.Join(root, "home")}
	tr.mkdir("home")
	return tr
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

func (tr *tree) symlink(rel, target string) {
	tr.t.Helper()
	tr.mkdir(filepath.ToSlash(filepath.Dir(filepath.FromSlash(rel))))
	if err := os.Symlink(target, tr.path(rel)); err != nil {
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
		"\nroute_scope: [none]\ntools: [Read]\nsuccess:\n  - { check: answer_delivered }\n"
	return s + strings.Join(extra, "\n") + map[bool]string{true: "\n", false: ""}[len(extra) > 0]
}

func layerSubagent(id, prompt string) string {
	return "id: " + id + "\nversion: 1\nstatus: approved\ntier: subagent\nmodel: claude-haiku-5-5\neffort: low\ndescription: helper\nprompt: " + prompt + "\n"
}

func TestParseUserConfig(t *testing.T) {
	tr := newTree(t)
	tr.mkdir("clients")
	tr.mkdir("home/work")
	tests := []struct {
		name    string
		yaml    string
		want    []string
		problem string
	}{
		{"absolute and tilde", "trusted_parents:\n  - " + tr.path("clients") + "\n  - ~/work\n", []string{tr.path("clients"), tr.path("home/work")}, ""},
		{"version 1 alone", "version: 1\n", nil, ""},
		{"no trusted parents", "trusted_parents: []\n", nil, ""},
		{"missing folder is skipped", "trusted_parents:\n  - " + tr.path("nowhere") + "\n", nil, ""},
		{"a repeat counts once", "trusted_parents:\n  - " + tr.path("clients") + "\n  - " + tr.path("clients") + "/.\n", []string{tr.path("clients")}, ""},
		{"top level list", "- a\n", nil, "the top level is not a mapping"},
		{"empty file", "", nil, "the top level is not a mapping"},
		{"yaml syntax", "a: [1\n", nil, ""}, // problem text is the decoder's
		{"unknown key", "colour: red\n", nil, `unknown key "colour"`},
		{"two unknown keys", "a: 1\nb: 2\n", nil, `unknown key "a"; unknown key "b"`},
		{"version 2", "version: 2\n", nil, "version must be 1"},
		{"version text", "version: \"1\"\n", nil, "version must be 1"},
		{"not a list", "trusted_parents: clients\n", nil, "trusted_parents must be a list of folders"},
		{"null list", "trusted_parents:\n", nil, "trusted_parents must be a list of folders"},
		{"list holding a number", "trusted_parents: [1]\n", nil, "trusted_parents must be a list of folders"},
		{"relative", "trusted_parents:\n  - clients\n", nil, `trusted parent "clients" must be an absolute path or start with ~/`},
		{"other user", "trusted_parents:\n  - ~bob/x\n", nil, `trusted parent "~bob/x" must be an absolute path or start with ~/`},
		{"root", "trusted_parents:\n  - /\n", nil, `trusted parent "/" is too broad (the filesystem root or HOME)`},
		{"root through dots", "trusted_parents:\n  - /a/../..\n", nil, `trusted parent "/a/../.." is too broad (the filesystem root or HOME)`},
		{"tilde", "trusted_parents:\n  - \"~\"\n", nil, `trusted parent "~" is too broad (the filesystem root or HOME)`},
		{"tilde slash", "trusted_parents:\n  - \"~/\"\n", nil, `trusted parent "~/" is too broad (the filesystem root or HOME)`},
		{"home", "trusted_parents:\n  - " + tr.home + "\n", nil, `trusted parent "` + tr.home + `" is too broad (the filesystem root or HOME)`},
		{"home with a trailing dot", "trusted_parents:\n  - ~/work/..\n", nil, `trusted parent "~/work/.." is too broad (the filesystem root or HOME)`},
		{"every problem is listed", "version: 3\ntrusted_parents:\n  - a\n  - /\n", nil,
			`version must be 1; trusted parent "a" must be an absolute path or start with ~/; trusted parent "/" is too broad (the filesystem root or HOME)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, problems := parseUserConfig([]byte(tt.yaml), tr.home)
			switch {
			case tt.problem != "":
				if got := strings.Join(problems, "; "); got != tt.problem {
					t.Errorf("problems %q, want %q", got, tt.problem)
				}
			case tt.name == "yaml syntax":
				if len(problems) != 1 {
					t.Errorf("want one decoder problem, got %v", problems)
				}
			default:
				if len(problems) != 0 {
					t.Fatalf("problems %v", problems)
				}
				if !reflect.DeepEqual(cfg.TrustedParents, tt.want) {
					t.Errorf("trusted %v, want %v", cfg.TrustedParents, tt.want)
				}
			}
		})
	}
}

// A symlink is trusted as the folder it leads to, and a symlink to HOME or to
// the root is as broad as HOME and the root.
func TestParseUserConfigSymlinks(t *testing.T) {
	tr := newTree(t)
	tr.mkdir("clients")
	tr.symlink("home/link", tr.path("clients"))
	tr.symlink("home/homelink", tr.home)
	tr.symlink("home/rootlink", "/")
	cfg, problems := parseUserConfig([]byte("trusted_parents:\n  - ~/link\n"), tr.home)
	if len(problems) != 0 || !reflect.DeepEqual(cfg.TrustedParents, []string{tr.path("clients")}) {
		t.Errorf("link: %v %v", cfg.TrustedParents, problems)
	}
	for _, entry := range []string{"~/homelink", "~/rootlink"} {
		_, problems := parseUserConfig([]byte("trusted_parents:\n  - "+entry+"\n"), tr.home)
		if want := `trusted parent "` + entry + `" is too broad (the filesystem root or HOME)`; strings.Join(problems, "; ") != want {
			t.Errorf("%s: %v", entry, problems)
		}
	}
}

func TestParentsOf(t *testing.T) {
	tr := newTree(t)
	outer := tr.mkdir("outer")
	mid := tr.mkdir("outer/mid")
	primary := tr.mkdir("outer/mid/repo")
	tr.mkdir("outer/mid/rep")      // shares the name prefix of repo
	tr.mkdir("outer/mid/repo-two") // a sibling that starts with repo
	tr.mkdir("elsewhere")
	tr.symlink("link-to-outer", outer)
	tests := []struct {
		name    string
		trusted []string
		want    []string
	}{
		{"none", nil, nil},
		{"a parent", []string{outer}, []string{outer}},
		{"nearest first whatever the file order", []string{outer, mid}, []string{mid, outer}},
		{"the same order reversed", []string{mid, outer}, []string{mid, outer}},
		{"the folder itself is not a parent", []string{primary}, nil},
		{"a sibling", []string{tr.path("outer/mid/repo-two")}, nil},
		{"a name prefix", []string{tr.path("outer/mid/rep")}, nil},
		{"a child", []string{tr.path("outer/mid/repo/src")}, nil},
		{"an unrelated folder", []string{tr.path("elsewhere")}, nil},
		{"a parent among others", []string{tr.path("elsewhere"), mid, tr.path("outer/mid/rep")}, []string{mid}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (UserConfig{TrustedParents: tt.trusted}).ParentsOf(primary); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}

	// Through the config the entry's spelling does not matter: a symlink, a
	// path with .. and a repeat all lead to the same real folder.
	for _, entry := range []string{"~/../link-to-outer", tr.path("link-to-outer"), outer + "/mid/..", outer + "/./"} {
		cfg, problems := parseUserConfig([]byte("trusted_parents:\n  - \""+entry+"\"\n"), tr.home)
		if len(problems) != 0 {
			t.Fatalf("%s: %v", entry, problems)
		}
		if got := cfg.ParentsOf(primary); !reflect.DeepEqual(got, []string{outer}) {
			t.Errorf("%s: parents %v, want %v", entry, got, []string{outer})
		}
	}
}

func TestLoadUserConfig(t *testing.T) {
	tr := newTree(t)
	if cfg, err := LoadUserConfig(tr.home); err != nil || len(cfg.TrustedParents) != 0 {
		t.Errorf("missing file: %v %v", cfg, err)
	}
	if cfg, err := LoadUserConfig(""); err != nil || len(cfg.TrustedParents) != 0 {
		t.Errorf("empty home: %v %v", cfg, err)
	}
	tr.mkdir("clients")
	tr.write("home/.jitso/config.yaml", "trusted_parents:\n  - "+tr.path("clients")+"\n")
	if cfg, err := LoadUserConfig(tr.home); err != nil || !reflect.DeepEqual(cfg.TrustedParents, []string{tr.path("clients")}) {
		t.Errorf("valid file: %v %v", cfg, err)
	}
	tr.write("home/.jitso/config.yaml", "trusted_parents:\n  - /\n")
	_, err := LoadUserConfig(tr.home)
	if want := `jitso: ~/.jitso/config.yaml is invalid: trusted parent "/" is too broad (the filesystem root or HOME)`; err == nil || fail.Text(err) != want || fail.Code(err) != fail.CodeEnv {
		t.Errorf("invalid file: %v (code %d)", err, fail.Code(err))
	}
}

func TestSources(t *testing.T) {
	tr := newTree(t)
	outer := tr.mkdir("outer")
	mid := tr.mkdir("outer/mid")
	primary := tr.mkdir("outer/mid/repo")
	inHome := tr.mkdir("home/clients")
	home := tr.home
	repoRepo := tr.mkdir("home/clients/app")

	got := Sources(primary, home, UserConfig{TrustedParents: []string{outer, mid}})
	want := []Source{
		{Layer: LayerRepo, Dir: filepath.Join(primary, "jitso", "builds"), Display: "jitso/builds"},
		{Layer: LayerParent, Dir: filepath.Join(mid, "jitso", "builds"), Display: filepath.ToSlash(mid) + "/jitso/builds"},
		{Layer: LayerParent, Dir: filepath.Join(outer, "jitso", "builds"), Display: filepath.ToSlash(outer) + "/jitso/builds"},
		{Layer: LayerUser, Dir: filepath.Join(home, ".jitso", "builds"), Display: "~/.jitso/builds"},
		{Layer: LayerDraft, Dir: filepath.Join(primary, ".jitso", "drafts"), Display: ".jitso/drafts"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sources\n got %+v\nwant %+v", got, want)
	}

	// A parent under HOME shows as ~/<path>.
	got = Sources(repoRepo, home, UserConfig{TrustedParents: []string{inHome}})
	if len(got) != 4 || got[1].Display != "~/clients/jitso/builds" || got[1].Dir != filepath.Join(inHome, "jitso", "builds") {
		t.Errorf("parent under home: %+v", got)
	}

	// No home: no user layer, and the draft layer is still last.
	if got := Sources(primary, "", UserConfig{}); len(got) != 2 || got[0].Layer != LayerRepo || got[1].Layer != LayerDraft {
		t.Errorf("no home: %+v", got)
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
// second is shadowed, in the project and in an outer layer alike.
func TestFirstFileWinsInsideALayer(t *testing.T) {
	for _, layer := range []Layer{LayerRepo, LayerUser} {
		t.Run(string(layer), func(t *testing.T) {
			dir := folder(layer, "dir", map[string]string{
				"z-dup.yaml": layerBuild("dup", "SECOND"),
				"a-dup.yaml": layerBuild("dup", "FIRST"),
			})
			sources := []Source{dir}
			if layer != LayerRepo {
				sources = []Source{folder(LayerRepo, "jitso/builds", nil), dir}
			}
			res := scan(t, sources...)
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
		})
	}
}

func TestScanShadowing(t *testing.T) {
	repo := folder(LayerRepo, "jitso/builds", map[string]string{
		"a.yaml": layerBuild("alpha", "repo alpha"),
		"b.yaml": layerBuild("beta", "repo beta"),
	})
	near := folder(LayerParent, "/near/jitso/builds", map[string]string{
		"alpha.yaml": layerBuild("alpha", "near alpha"),
		"gamma.yaml": layerBuild("gamma", "near gamma"),
	})
	far := folder(LayerParent, "/far/jitso/builds", map[string]string{
		"gamma.yaml": layerBuild("gamma", "far gamma"),
		"delta.yaml": layerBuild("delta", "far delta"),
	})
	user := folder(LayerUser, "~/.jitso/builds", map[string]string{
		"delta.yaml": layerBuild("delta", "user delta"),
		"eps.yaml":   layerBuild("epsilon", "user epsilon"),
	})
	res := scan(t, repo, near, far, user)
	want := []string{"repo:a.yaml", "repo:b.yaml", "parent:alpha.yaml", "parent:gamma.yaml", "parent:delta.yaml", "parent:gamma.yaml", "user:delta.yaml", "user:eps.yaml"}
	if got := ids(res.Files); !reflect.DeepEqual(got, want) {
		t.Fatalf("files %v", got)
	}
	shadowed := map[string]string{}
	for _, f := range res.Files {
		if f.ShadowedBy != nil {
			shadowed[f.Display] = f.ShadowedBy.Display
		}
	}
	wantShadowed := map[string]string{
		"/near/jitso/builds/alpha.yaml": "jitso/builds/a.yaml",
		"/far/jitso/builds/gamma.yaml":  "/near/jitso/builds/gamma.yaml",
		"~/.jitso/builds/delta.yaml":    "/far/jitso/builds/delta.yaml",
	}
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
	wantBuilds := []string{"alpha/repo/repo alpha", "beta/repo/repo beta", "gamma/parent/near gamma", "delta/parent/far delta", "epsilon/user/user epsilon"}
	if !reflect.DeepEqual(got, wantBuilds) {
		t.Errorf("builds %v\nwant   %v", got, wantBuilds)
	}
	lines := strings.Split(cat.List(), "\n")
	if lines[0] != "- alpha: claude-haiku-5-5 low. repo alpha requires: nothing" ||
		lines[2] != "- gamma: claude-haiku-5-5 low. near gamma requires: nothing (from parent /near/jitso/builds/gamma.yaml)" ||
		lines[4] != "- epsilon: claude-haiku-5-5 low. user epsilon requires: nothing (from user ~/.jitso/builds/eps.yaml)" {
		t.Errorf("list lines:\n%s", cat.List())
	}
}

// The nearest file of an id is the build whether or not it is valid here: an
// invalid parent file is skipped, and the farther valid file is not used.
func TestInvalidWinnerBlocksFartherFiles(t *testing.T) {
	bad := strings.Replace(layerBuild("shared", "near"), "claude-haiku-5-5", "gpt", 1)
	near := folder(LayerParent, "/near/jitso/builds", map[string]string{"shared.yaml": bad})
	user := folder(LayerUser, "~/.jitso/builds", map[string]string{"shared.yaml": layerBuild("shared", "user"), "ok.yaml": layerBuild("ok", "fine")})
	cat, err := scan(t, folder(LayerRepo, "jitso/builds", nil), near, user).Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if _, found := cat.Find("shared"); found {
		t.Error("the farther valid file must not be used")
	}
	s, invalid := cat.Invalid("shared")
	if !invalid || s.Layer != LayerParent || s.Display != "/near/jitso/builds/shared.yaml" || !reflect.DeepEqual(s.Problems, []string{`model "gpt" is not in the allowlist`}) {
		t.Fatalf("invalid: %+v %v", s, invalid)
	}
	if len(cat.Skipped) != 1 {
		t.Errorf("only the winner is reported, got %+v", cat.Skipped)
	}
	if got := cat.Skipped[0].Warning(); got != `jitso: warning: skipped build parent /near/jitso/builds/shared.yaml: model "gpt" is not in the allowlist` {
		t.Errorf("warning %q", got)
	}
	if _, found := cat.Find("ok"); !found {
		t.Error("the other user build must stay")
	}
	if _, invalid := cat.Invalid("ok"); invalid {
		t.Error("ok is not invalid")
	}
}

func TestOuterFilesInvalidHereAreSkipped(t *testing.T) {
	user := folder(LayerUser, "~/.jitso/builds", map[string]string{
		"a-broken.yaml":     "id: [",
		"b-noid.yaml":       "version: 1\n",
		"c-bad-id.yaml":     strings.Replace(layerBuild("x", "d"), "id: x", "id: Not Kebab", 1),
		"d-route.yaml":      strings.Replace(layerBuild("route", "d"), "route_scope: [none]", "route_scope: [elsewhere]", 1),
		"e-missing.yaml":    layerBuild("lost", "d", "subagents: [nobody]"),
		"f-worker.yaml":     layerBuild("worker-sub", "d", "subagents: [worker-tier]"),
		"g-workertier.yaml": layerBuild("worker-tier", "d"),
		"h-fine.yaml":       layerBuild("fine", "d"),
		"i-notyaml.txt":     "ignored",
	})
	cat, err := scan(t, folder(LayerRepo, "jitso/builds", nil), user).Catalog()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, s := range cat.Skipped {
		got[s.Display] = s.ID + "|" + strings.Join(s.Problems, "; ")
	}
	for display, want := range map[string]string{
		"~/.jitso/builds/b-noid.yaml":    "|id must be kebab-case",
		"~/.jitso/builds/c-bad-id.yaml":  "|id must be kebab-case",
		"~/.jitso/builds/d-route.yaml":   `route|route_scope id "elsewhere" is not an instruction section`,
		"~/.jitso/builds/e-missing.yaml": `lost|subagent "nobody" is not a subagent-tier build`,
		"~/.jitso/builds/f-worker.yaml":  `worker-sub|subagent "worker-tier" is not a subagent-tier build`,
	} {
		if display == "~/.jitso/builds/b-noid.yaml" {
			if !strings.HasPrefix(got[display], want) {
				t.Errorf("%s: got %q, want it to start with %q", display, got[display], want)
			}
			continue
		}
		if got[display] != want {
			t.Errorf("%s: got %q, want %q", display, got[display], want)
		}
	}
	if !strings.HasPrefix(got["~/.jitso/builds/a-broken.yaml"], "|") || len(got) != 6 {
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
	user := folder(LayerUser, "~/.jitso/builds", map[string]string{"user-bad.yaml": bad})
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
			_, err := scan(t, folder(LayerRepo, "jitso/builds", tt.files), user).Catalog()
			var fe *FileError
			if !errors.As(err, &fe) || err.Error() != tt.want {
				t.Errorf("got %v, want %s", err, tt.want)
			}
		})
	}
}

// A worker of any layer resolves its subagents with the same lookup: the
// nearest file of the name, so the project's plan beats the user's.
func TestSubagentsResolveThroughTheLayers(t *testing.T) {
	repo := folder(LayerRepo, "jitso/builds", map[string]string{"plan.yaml": layerSubagent("plan", "REPO PLAN")})
	user := folder(LayerUser, "~/.jitso/builds", map[string]string{
		"plan.yaml":   layerSubagent("plan", "USER PLAN"),
		"helper.yaml": layerSubagent("helper", "USER HELPER"),
		"team.yaml":   layerBuild("team", "d", "subagents: [plan, helper]"),
	})
	cat, err := scan(t, repo, user).Catalog()
	if err != nil {
		t.Fatal(err)
	}
	team, _ := cat.Find("team")
	subs := cat.SubagentsOf(team)
	if len(subs) != 2 || subs[0].Prompt != "REPO PLAN" || subs[0].Layer != LayerRepo || subs[1].Prompt != "USER HELPER" || subs[1].Layer != LayerUser {
		t.Errorf("subagents %+v", subs)
	}

	// A subagent whose nearest file is invalid here makes the worker invalid.
	badRepoPlan := strings.Replace(layerSubagent("plan", "x"), "claude-haiku-5-5", "gpt", 1)
	user.fsys.(fstest.MapFS)["extra.yaml"] = &fstest.MapFile{Data: []byte(layerBuild("extra", "d", "subagents: [plan]"))}
	parent := folder(LayerParent, "/p/jitso/builds", map[string]string{"plan.yaml": badRepoPlan})
	cat, err = scan(t, folder(LayerRepo, "jitso/builds", nil), parent, user).Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := cat.Invalid("extra"); !ok || s.Problems[0] != `subagent "plan" is not a subagent-tier build` {
		t.Errorf("extra: %+v %v", s, ok)
	}
}

func TestListLineOrigin(t *testing.T) {
	raw := with(t).(*Map)
	b := buildFrom("x.yaml", raw)
	if got := ListLine(&b); got != SummaryLine(&b) {
		t.Errorf("a build made by hand reads as repo: %q", got)
	}
	b.Layer, b.Display = LayerRepo, "jitso/builds/x.yaml"
	if got := ListLine(&b); got != SummaryLine(&b) {
		t.Errorf("repo: %q", got)
	}
	b.Layer, b.Display = LayerUser, "~/.jitso/builds/x.yaml"
	if got := ListLine(&b); got != SummaryLine(&b)+" (from user ~/.jitso/builds/x.yaml)" {
		t.Errorf("user: %q", got)
	}
}

// LoadLayers over real folders: the layers come from the primary's real path
// and the user config, the project reads its own jitso.yaml and never a parent's.
func TestLoadLayers(t *testing.T) {
	tr := newTree(t)
	tr.project("outer")
	tr.write("outer/jitso/jitso.yaml", "version: 1\nmodels: [claude-opus-9]\n") // never read for the nested project
	tr.write("outer/jitso/builds/shared.yaml", layerBuild("shared", "from the parent"))
	tr.write("outer/jitso/builds/only-parent.yaml", layerBuild("only-parent", "parent"))
	primary := tr.project("outer/nested")
	tr.write("outer/nested/jitso/builds/mine.yaml", layerBuild("mine", "repo"))
	tr.write("home/.jitso/builds/shared.yaml", layerBuild("shared", "from the user"))
	tr.write("home/.jitso/builds/user-only.yaml", layerBuild("user-only", "user"))
	tr.write("home/.jitso/config.yaml", "trusted_parents:\n  - "+tr.path("outer")+"\n")

	cat, err := LoadLayers(os.DirFS(primary), primary, tr.home)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, b := range cat.Builds {
		got = append(got, string(b.Layer)+":"+b.ID+":"+b.Display)
	}
	want := []string{
		"repo:mine:jitso/builds/mine.yaml",
		"parent:only-parent:" + filepath.ToSlash(tr.path("outer")) + "/jitso/builds/only-parent.yaml",
		"parent:shared:" + filepath.ToSlash(tr.path("outer")) + "/jitso/builds/shared.yaml",
		"user:user-only:~/.jitso/builds/user-only.yaml",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("builds\n got %v\nwant %v", got, want)
	}
	shared, _ := cat.Find("shared")
	if shared.Path != tr.path("outer/jitso/builds/shared.yaml") || len(shared.SHA256) != 64 {
		t.Errorf("shared: path %q sha %q", shared.Path, shared.SHA256)
	}

	// No user config: only the project and the user folder.
	if err := os.Remove(tr.path("home/.jitso/config.yaml")); err != nil {
		t.Fatal(err)
	}
	cat, err = LoadLayers(os.DirFS(primary), primary, tr.home)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(cat.Builds); got != 3 {
		t.Errorf("without trust: %d builds", got)
	}
	if b, _ := cat.Find("shared"); b.Layer != LayerUser {
		t.Errorf("without trust shared is %s", b.Layer)
	}

	// The project's own trust file is never read.
	tr.write("outer/nested/.jitso/config.yaml", "trusted_parents:\n  - "+tr.path("outer")+"\n")
	cat, _ = LoadLayers(os.DirFS(primary), primary, tr.home)
	if _, found := cat.Find("only-parent"); found {
		t.Error("a project cannot declare trust")
	}

	// The SHA-256 is that of the file's bytes.
	data, _ := os.ReadFile(tr.path("home/.jitso/builds/user-only.yaml"))
	b, _ := cat.Find("user-only")
	if b.SHA256 != sha256Hex(data) {
		t.Errorf("sha %s", b.SHA256)
	}
}

func TestLoadLayersMissingPieces(t *testing.T) {
	tr := newTree(t)
	primary := tr.mkdir("repo")
	if _, err := LoadLayers(os.DirFS(primary), primary, tr.home); err == nil || fail.Text(err) != "jitso: jitso/jitso.yaml is missing: this project has no JITSO allowlists (create it with jitso build new <id>)" || fail.Code(err) != fail.CodeEnv {
		t.Errorf("no jitso.yaml: %v", err)
	}
	tr.project("repo")
	tr.write("home/.jitso/builds/u.yaml", layerBuild("u", "user"))
	cat, err := LoadLayers(os.DirFS(primary), primary, tr.home)
	if err != nil || len(cat.Builds) != 1 || cat.Builds[0].Layer != LayerUser {
		t.Errorf("no jitso/builds folder is an empty project layer: %v %+v", err, cat)
	}
	tr.write("home/.jitso/config.yaml", "colour: red\n")
	if _, err := LoadLayers(os.DirFS(primary), primary, tr.home); err == nil || fail.Code(err) != fail.CodeEnv || !strings.Contains(err.Error(), `~/.jitso/config.yaml is invalid: unknown key "colour"`) {
		t.Errorf("invalid user config: %v", err)
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
	res, err := Scan(fsys, "", "")
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
