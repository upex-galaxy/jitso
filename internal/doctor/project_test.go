package doctor

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/upex-galaxy/jitso/internal/project"
	"github.com/upex-galaxy/jitso/internal/registry"
)

// fixture is a copy of the contract's default project in a temp folder (no
// git: the git questions go through a stand-in Runner).
func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("../../contract/fixtures/default")); err != nil {
		t.Fatal(err)
	}
	return project.Real(dir)
}

func put(t *testing.T, dir, rel, text string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// checksFor is the project checks over dir, with git answering by the table.
func checksFor(t *testing.T, dir string, git map[string]int) (*projectChecks, *script) {
	t.Helper()
	s := &script{answers: map[string]Result{}}
	for args, exit := range git {
		s.answers["git "+args] = Result{Exit: exit}
	}
	p := Ports{GOOS: runtime.GOOS, Version: "v9.9.9", Home: t.TempDir(), Run: s.run, Now: time.Now}.withDefaults()
	return &projectChecks{p: p, root: project.Root{Primary: dir, Checkout: dir}}, s
}

func TestCatalog(t *testing.T) {
	dir := fixture(t)
	c, _ := checksFor(t, dir, nil)
	got := c.catalog()
	want(t, got, OK, "catalog", "jitso/jitso.yaml, 7 builds, 4 launchable")

	put(t, dir, "jitso/jitso.yaml", "models: [\n")
	c, _ = checksFor(t, dir, nil)
	want(t, c.catalog(), Fail, "catalog", "jitso/jitso.yaml is invalid")

	if err := os.Remove(filepath.Join(dir, "jitso/jitso.yaml")); err != nil {
		t.Fatal(err)
	}
	// Without the file every list is open, so the skills the builds name must
	// exist: a warning about the file, not a failure.
	for _, skill := range []string{"orca-orchestration", "sprint-testing", "test-automation"} {
		put(t, dir, ".agents/skills/"+skill+"/SKILL.md", "---\nname: "+skill+"\n---\n")
	}
	c, _ = checksFor(t, dir, nil)
	want(t, c.catalog(), Warn, "catalog", "no jitso/jitso.yaml: every allowlist is open and the guard stays off in this project (run jitso init); 7 builds, 4 launchable")

	put(t, dir, "jitso/jitso.yaml", "models: [\n")
	c, _ = checksFor(t, dir, nil)
	c.catalog()
	want(t, c.mcp(), Warn, "mcp", "not checked: the catalog did not load")
}

func TestCatalogErrorKeepsTheFileName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports reading a file as a folder as a missing path, which counts as no route ids")
	}
	dir := fixture(t)
	if err := os.RemoveAll(filepath.Join(dir, ".agents/instructions")); err != nil {
		t.Fatal(err)
	}
	// A missing folder is no route ids (route_scope is optional); a path that
	// cannot be read as a folder is an error that names it.
	put(t, dir, ".agents/instructions", "not a folder\n")
	c, _ := checksFor(t, dir, nil)
	want(t, c.catalog(), Fail, "catalog", ".agents/instructions")
}

func TestCatalogNamesSkippedDrafts(t *testing.T) {
	dir := fixture(t)
	c, _ := checksFor(t, dir, nil)
	// A draft that is invalid here is skipped, and doctor says which.
	put(t, dir, ".jitso/drafts/mine.yaml", "id: mine\nversion: 1\nstatus: draft\n")
	got := c.catalog()
	want(t, got, Warn, "catalog", "jitso/jitso.yaml, 7 builds, 4 launchable; skipped draft .jitso/drafts/mine.yaml (")
}

func TestMCP(t *testing.T) {
	dir := fixture(t)
	c, _ := checksFor(t, dir, nil)
	c.catalog()
	want(t, c.mcp(), OK, "mcp", ".mcp.json: 3 servers")

	// A launchable build lists servers: the file must exist, parse and define them.
	if err := os.Remove(filepath.Join(dir, ".mcp.json")); err != nil {
		t.Fatal(err)
	}
	want(t, c.mcp(), Fail, "mcp", ".mcp.json is missing, and builds list MCP servers (")
	put(t, dir, ".mcp.json", "{")
	want(t, c.mcp(), Fail, "mcp", ".mcp.json is not valid JSON")
	put(t, dir, ".mcp.json", "null")
	want(t, c.mcp(), Fail, "mcp", ".mcp.json is null")
	put(t, dir, ".mcp.json", `{"mcpServers": {"dbhub": {}}}`)
	want(t, c.mcp(), Fail, "mcp", ".mcp.json does not define openapi")
}

func TestMCPWhenNoBuildListsAServer(t *testing.T) {
	dir := fixture(t)
	// Every build of the fixture that lists a server goes: nothing needs the file.
	entries, _ := os.ReadDir(filepath.Join(dir, "jitso/builds"))
	for _, e := range entries {
		path := filepath.Join(dir, "jitso/builds", e.Name())
		data, _ := os.ReadFile(path)
		var out []string
		for _, l := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(l, "mcp:") {
				l = "mcp: []"
			}
			out = append(out, l)
		}
		os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644)
	}
	os.Remove(filepath.Join(dir, ".mcp.json"))
	c, _ := checksFor(t, dir, nil)
	if got := c.catalog(); got.Status == Fail {
		t.Fatalf("catalog: %s", got)
	}
	want(t, c.mcp(), OK, "mcp", "no build lists an MCP server")
	put(t, dir, ".mcp.json", "{")
	want(t, c.mcp(), Warn, "mcp", ".mcp.json is not valid JSON")
}

func TestState(t *testing.T) {
	dir := fixture(t)
	ignored := "check-ignore -q .jitso/launches.jsonl"
	c, s := checksFor(t, dir, map[string]int{ignored: 0})
	want(t, c.state(), OK, "state", ".jitso/ is ignored by git")
	if len(s.calls) != 1 || s.calls[0] != "git "+ignored {
		t.Errorf("calls = %v: only check-ignore may run", s.calls)
	}
	c, _ = checksFor(t, dir, map[string]int{ignored: 1})
	want(t, c.state(), Warn, "state", ".jitso/ is not ignored by git: add .jitso/ to .gitignore")
	c, _ = checksFor(t, dir, map[string]int{ignored: 128})
	want(t, c.state(), Warn, "state", "git could not say")
	c, _ = checksFor(t, dir, nil) // the runner errors
	want(t, c.state(), Warn, "state", "git could not say")
}

func TestRegistry(t *testing.T) {
	dir := fixture(t)
	c, _ := checksFor(t, dir, nil)
	want(t, c.registry(), OK, "registry", "no launches yet")

	store := registry.NewFileStore(c.root.StateDir())
	const ts = "2026-11-13T14:22:33.000Z"
	launch := func(id string) *registry.LaunchEvent {
		return &registry.LaunchEvent{ID: id, Root: id, TS: ts, Mode: "direct", Build: "b", BuildVersion: 1,
			Orca: registry.OrcaRef{Terminal: "t", Title: "[jitso:" + id + "] b"}}
	}
	a, b := "L20261113T142233-aaaa", "L20261113T142233-bbbb"
	for _, r := range []registry.Record{launch(a), launch(b), &registry.EndEvent{ID: a, TS: ts, Source: "jitso_close", OutcomeReported: "none"}} {
		if err := store.Append(r); err != nil {
			t.Fatal(err)
		}
	}
	want(t, c.registry(), OK, "registry", ".jitso/launches.jsonl: 2 launches, 1 running")

	f, _ := os.OpenFile(store.Path(), os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("not json\n\n{\"event\":\"nope\"}\n")
	f.Close()
	want(t, c.registry(), Warn, "registry", "2 launches, 1 running; 2 invalid lines skipped")

	// Lines written at schema v1 get their own clause, counted apart from the other invalid lines.
	const v1 = `{"v":1,"event":"launch","id":"L20261113T142233-cccc","parent":null,"root":"L20261113T142233-cccc","depth":0,"ts":"` + ts + `","mode":"direct","preset":"b"}` + "\n"
	f, _ = os.OpenFile(store.Path(), os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString(v1)
	f.Close()
	want(t, c.registry(), Warn, "registry", "2 launches, 1 running; 1 v1 line skipped (written before the log moved to v: 2); 2 invalid lines skipped")
	f, _ = os.OpenFile(store.Path(), os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString(v1)
	f.Close()
	want(t, c.registry(), Warn, "registry", "2 launches, 1 running; 2 v1 lines skipped (written before the log moved to v: 2); 2 invalid lines skipped")

	// A path that is a folder cannot be read.
	os.Remove(store.Path())
	os.Mkdir(store.Path(), 0o755)
	want(t, c.registry(), Fail, "registry", ".jitso/launches.jsonl cannot be read")
}

func TestRegistryOnlyV1Lines(t *testing.T) {
	dir := fixture(t)
	c, _ := checksFor(t, dir, nil)
	put(t, dir, ".jitso/launches.jsonl", `{"event":"launch","v":1,"id":"L20261113T142233-cccc","preset":"b"}`+"\n")
	want(t, c.registry(), Warn, "registry", ".jitso/launches.jsonl: 0 launches, 0 running; 1 v1 line skipped (written before the log moved to v: 2)")
}

func TestDrafts(t *testing.T) {
	dir := fixture(t)
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	ignoredCmd := "check-ignore -q .jitso/drafts/quick.yaml"
	c, _ := checksFor(t, dir, map[string]int{ignoredCmd: 0})
	c.p.Now = func() time.Time { return now }
	want(t, c.drafts(), OK, "drafts", "no drafts")

	put(t, dir, ".jitso/drafts/quick.yaml", "id: quick\n")
	put(t, dir, ".jitso/drafts/NOT_A_DRAFT.txt", "x")
	draft := filepath.Join(dir, ".jitso/drafts/quick.yaml")
	age := func(d time.Duration) {
		t.Helper()
		if err := os.Chtimes(draft, now.Add(-d), now.Add(-d)); err != nil {
			t.Fatal(err)
		}
	}

	age(2 * time.Hour)
	want(t, c.drafts(), OK, "drafts", "1 draft")
	age(7 * 24 * time.Hour) // exactly 7 days is not more than 7 days
	want(t, c.drafts(), OK, "drafts", "1 draft")
	age(7*24*time.Hour + time.Minute)
	want(t, c.drafts(), Warn, "drafts", "1 draft, the oldest is 7 days old")
	age(10*24*time.Hour + 5*time.Hour)
	want(t, c.drafts(), Warn, "drafts", "1 draft, the oldest is 10 days old")

	// The oldest of several decides; a draft folder git does not ignore is its own warning.
	put(t, dir, ".jitso/drafts/fresh.yaml", "id: fresh\n")
	os.Chtimes(filepath.Join(dir, ".jitso/drafts/fresh.yaml"), now, now)
	c, _ = checksFor(t, dir, map[string]int{"check-ignore -q .jitso/drafts/fresh.yaml": 1})
	c.p.Now = func() time.Time { return now }
	got := c.drafts()
	want(t, got, Warn, "drafts", ".jitso/drafts/ is not ignored by git; 2 drafts, the oldest is 10 days old")
}

func TestBackups(t *testing.T) {
	dir := fixture(t)
	c, _ := checksFor(t, dir, nil)
	want(t, c.backups(), OK, "backups", "no backups")
	put(t, dir, ".jitso/backups/.gitignore", "*\n")
	want(t, c.backups(), OK, "backups", "no backups")
	put(t, dir, ".jitso/backups/20261115T120000Z-ab12/.claude/settings.json", "{}")
	put(t, dir, ".jitso/backups/20261115T120001Z-cd34/AGENTS.md", "x")
	want(t, c.backups(), OK, "backups", "2 backups in .jitso/backups/")
}
