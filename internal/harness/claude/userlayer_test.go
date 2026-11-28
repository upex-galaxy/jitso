package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/harness"
)

func put(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, text := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUserLayerOverrides(t *testing.T) {
	home := t.TempDir()
	checkout := t.TempDir()
	put(t, home, map[string]string{
		".claude/settings.json":           `{"enabledPlugins":{"a@m":true,"b@m":false},"env":{"X":"sentinel"},"language":"sentinel"}`,
		".claude/CLAUDE.md":               "# n\n@~/notes/one.md\n@rules/two.md and @missing.md\n@rules/two.md\nmail me@example.com `@code.md`\n```\n@fenced.md\n```\n@~/notes/dir\n",
		"notes/one.md":                    "1",
		".claude/rules/two.md":            "2",
		".claude/code.md":                 "x",
		".claude/fenced.md":               "x",
		"notes/dir/inner.md":              "x",
		".claude/skills/s1/SKILL.md":      "x",
		".claude/skills/jitso/SKILL.md":   "x",
		".claude/skills/.hidden/SKILL.md": "x",
		".claude/skills/file.txt":         "not a folder",
		".agents/skills/s2/SKILL.md":      "x",
		".claude/agents/a.md":             "---\nname: \"named-a\"\n---\nbody",
		".claude/agents/b.md":             "no frontmatter",
		".claude/agents/c.txt":            "not an agent",
	})
	put(t, checkout, map[string]string{".claude/skills/s2/SKILL.md": "x"})
	got := Adapter{}.UserLayer(harness.UserLayerInput{
		Home: home, LookupEnv: noEnv, Checkout: checkout, Servers: []string{"dbhub"},
		Connectors: []harness.CloudConnector{{Name: "Supabase", URL: "https://mcp.supabase.com/mcp"}},
		KeepSkills: []string{"jitso"},
	})
	if got.Warning != "" || got.GuardRegistered {
		t.Errorf("warning %q guard %v", got.Warning, got.GuardRegistered)
	}
	if want := []string{"allowedMcpServers", "claudeMdExcludes", "enabledPlugins", "permissions", "remoteControlAtStartup", "skillOverrides"}; !reflect.DeepEqual(got.Overrides, want) {
		t.Errorf("overrides %v", got.Overrides)
	}
	if !reflect.DeepEqual(got.Cloud, []string{"Supabase"}) {
		t.Errorf("cloud %v", got.Cloud)
	}
	data, _ := json.Marshal(got.Doc)
	var doc map[string]any
	_ = json.Unmarshal(data, &doc)
	claude := filepath.Join(home, ".claude")
	want := map[string]any{
		"allowedMcpServers":      []any{map[string]any{"serverName": "dbhub"}, map[string]any{"serverUrl": "https://mcp.supabase.com/mcp"}},
		"claudeMdExcludes":       []any{filepath.Join(claude, "CLAUDE.md"), filepath.Join(home, "notes", "one.md"), filepath.Join(claude, "rules", "two.md")},
		"enabledPlugins":         map[string]any{"a@m": false, "b@m": false},
		"permissions":            map[string]any{"deny": []any{"Agent(b)", "Agent(named-a)"}},
		"remoteControlAtStartup": false,
		"skillOverrides":         map[string]any{"s1": "off"},
	}
	if !reflect.DeepEqual(doc, want) {
		t.Errorf("doc\n got %v\nwant %v", doc, want)
	}
	if strings.Contains(string(data), "sentinel") {
		t.Error("a value of the owner's settings reached the overrides")
	}
}

func TestUserLayerEmptyFolders(t *testing.T) {
	got := Adapter{}.UserLayer(harness.UserLayerInput{Home: t.TempDir(), LookupEnv: noEnv})
	if want := []string{"allowedMcpServers", "claudeMdExcludes", "remoteControlAtStartup"}; !reflect.DeepEqual(got.Overrides, want) {
		t.Errorf("overrides %v", got.Overrides)
	}
	if list, _ := got.Doc["allowedMcpServers"].([]any); list == nil || len(list) != 0 {
		t.Errorf("allowedMcpServers must be an empty list, got %#v", got.Doc["allowedMcpServers"])
	}
	// No home and no config dir: nothing to read, the two keys of the launch.
	got = Adapter{}.UserLayer(harness.UserLayerInput{LookupEnv: noEnv})
	if want := []string{"allowedMcpServers", "remoteControlAtStartup"}; !reflect.DeepEqual(got.Overrides, want) {
		t.Errorf("no home: overrides %v", got.Overrides)
	}
}

func TestUserLayerConfigDirAndGuard(t *testing.T) {
	home, cfg := t.TempDir(), t.TempDir()
	put(t, home, map[string]string{".claude/skills/ignored/SKILL.md": "x"})
	const guard = `{"type":"command","command":"jitso hook guard"}`
	put(t, cfg, map[string]string{
		"settings.json":       `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[` + guard + `]},{"matcher":"mcp__.*orca.*__.*","hooks":[` + guard + `]}],"SessionEnd":[{"hooks":[` + guard + `]}]}}`,
		"skills/own/SKILL.md": "x",
	})
	env := func(k string) (string, bool) { return cfg, k == ConfigDirEnv }
	got := Adapter{}.UserLayer(harness.UserLayerInput{Home: home, LookupEnv: env})
	if !got.GuardRegistered {
		t.Error("the guard on all three slots is registered")
	}
	if skills, _ := got.Doc["skillOverrides"].(map[string]any); !reflect.DeepEqual(skills, map[string]any{"own": "off"}) {
		t.Errorf("skills %v", skills)
	}
	if list, _ := got.Doc["claudeMdExcludes"].([]any); len(list) != 1 || list[0] != filepath.Join(cfg, "CLAUDE.md") {
		t.Errorf("claudeMdExcludes %v", got.Doc["claudeMdExcludes"])
	}
	// One slot of three is not the guard.
	put(t, cfg, map[string]string{"settings.json": `{"hooks":{"SessionEnd":[{"hooks":[` + guard + `]}]}}`})
	if partial := (Adapter{}).UserLayer(harness.UserLayerInput{Home: home, LookupEnv: env}); partial.GuardRegistered {
		t.Error("a partial guard is not registered")
	}
}

func TestUserLayerInvalidSettingsWarns(t *testing.T) {
	home := t.TempDir()
	put(t, home, map[string]string{".claude/settings.json": "{not json sentinel"})
	got := Adapter{}.UserLayer(harness.UserLayerInput{Home: home, LookupEnv: noEnv})
	if !strings.Contains(got.Warning, filepath.Join(home, ".claude", "settings.json")) || strings.Contains(got.Warning, "sentinel") {
		t.Errorf("warning %q", got.Warning)
	}
	if _, ok := got.Doc["enabledPlugins"]; ok {
		t.Error("no plugin to switch off")
	}
}

func TestArgvUserLayer(t *testing.T) {
	in := harness.SpecInput{Model: "m", Effort: "e", SessionID: "s", LaunchDir: filepath.Join("l"), MCPConfigPath: filepath.Join("l", "mcp.json"),
		UserLayer: &harness.UserLayer{Doc: map[string]any{"remoteControlAtStartup": false}}}
	argv, err := Argv(in)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"claude", "--dangerously-skip-permissions", "--setting-sources", "user,project", "--settings", filepath.Join("l", "settings.json"),
		"--mcp-config", filepath.Join("l", "mcp.json"), "--model", "m", "--effort", "e", "--session-id", "s"}
	if !reflect.DeepEqual(argv, want) {
		t.Errorf("got  %q\nwant %q", argv, want)
	}
	spec, err := Adapter{}.Spec(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Files) != 1 || spec.Files[0].Mode != 0o600 {
		t.Fatalf("files %+v", spec.Files)
	}
	var doc map[string]any
	if err := json.Unmarshal(spec.Files[0].Data, &doc); err != nil || doc["remoteControlAtStartup"] != false || doc["skipDangerousModePermissionPrompt"] != true {
		t.Errorf("settings %s, %v", spec.Files[0].Data, err)
	}
}
