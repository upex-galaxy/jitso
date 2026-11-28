package claude

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/harness"
	"github.com/upex-galaxy/jitso/internal/jsonx"
)

func TestArgvName(t *testing.T) {
	in := harness.SpecInput{Model: "m", Effort: "low", SessionID: "sid", MCPConfigPath: "/l/mcp.json", Name: "[jitso:L1] build one", Tools: []string{"Read"}, ToolsSet: true}
	got, err := Argv(in)
	if err != nil {
		t.Fatal(err)
	}
	// The name is one word right after the session id, before the tool list.
	at := slices.Index(got, "--session-id")
	if at < 0 || !slices.Equal(got[at+2:at+4], []string{"--name", "[jitso:L1] build one"}) || got[at+4] != "--tools=Read" {
		t.Errorf("got %q", got)
	}
	in.Name = ""
	got, _ = Argv(in)
	if slices.Contains(got, "--name") {
		t.Errorf("an empty name still passed --name: %q", got)
	}
}

func TestArgv(t *testing.T) {
	base := harness.SpecInput{Model: "claude-haiku-5-5", Effort: "low", SessionID: "sid", MCPConfigPath: "/l/mcp.json", LaunchDir: "/l"}
	head := []string{"claude", "--dangerously-skip-permissions", "--setting-sources", "project", "--settings", filepath.Join("/l", "settings.json"),
		"--strict-mcp-config", "--mcp-config", "/l/mcp.json", "--model", "claude-haiku-5-5", "--effort", "low",
		"--session-id", "sid"}
	tests := []struct {
		name string
		edit func(*harness.SpecInput)
		tail []string
	}{
		{"fleet, harness tools", func(*harness.SpecInput) {}, nil},
		{"tools", func(in *harness.SpecInput) { in.Tools, in.ToolsSet = []string{"Read", "Bash"}, true }, []string{"--tools=Read,Bash"}},
		{"empty tools", func(in *harness.SpecInput) { in.ToolsSet = true }, []string{"--tools="}},
		{"direct prompt", func(in *harness.SpecInput) {
			in.Tools, in.ToolsSet, in.InitialPrompt = []string{"Read"}, true, "Read /b.md and follow it exactly."
		}, []string{"--tools=Read", "Read /b.md and follow it exactly."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := base
			tt.edit(&in)
			got, err := Argv(in)
			if err != nil {
				t.Fatal(err)
			}
			if want := append(append([]string(nil), head...), tt.tail...); !reflect.DeepEqual(got, want) {
				t.Errorf("got  %q\nwant %q", got, want)
			}
		})
	}
}

func TestMCPConfig(t *testing.T) {
	repo := map[string]any{
		"dbhub":   map[string]any{"command": "fixture-dbhub", "args": []any{"--x", 1.0}, "env": map[string]any{"SECRET": "v"}},
		"openapi": map[string]any{"command": "fixture-openapi"},
		"broken":  map[string]any{"args": []any{}},
	}
	cfg, err := MCPConfig([]string{"openapi", "dbhub"}, repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := jsonx.MarshalIndent(cfg, "  ")
	want := "{\n  \"mcpServers\": {\n    \"openapi\": {\n      \"command\": \"fixture-openapi\",\n      \"args\": []\n    },\n" +
		"    \"dbhub\": {\n      \"command\": \"fixture-dbhub\",\n      \"args\": [\n        \"--x\",\n        \"1\"\n      ]\n    }\n  }\n}"
	if string(got) != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	empty, _ := MCPConfig(nil, nil, nil)
	if got, _ := jsonx.Marshal(empty); string(got) != `{"mcpServers":{}}` {
		t.Errorf("empty %s", got)
	}
	for _, name := range []string{"missing", "broken"} {
		if _, err := MCPConfig([]string{name}, repo, nil); err == nil || err.Error() != `MCP server "`+name+`" is not in the repo MCP config` {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestNewSessionID(t *testing.T) {
	id, err := Adapter{}.NewSessionID(bytes.NewReader(bytes.Repeat([]byte{0xff}, 16)))
	if err != nil {
		t.Fatal(err)
	}
	if id != "ffffffff-ffff-4fff-bfff-ffffffffffff" {
		t.Errorf("got %s", id)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(id) {
		t.Errorf("not a v4 uuid: %s", id)
	}
	if _, err := (Adapter{}).NewSessionID(bytes.NewReader(nil)); err == nil {
		t.Error("a short reader must fail")
	}
}

func TestTranscriptPath(t *testing.T) {
	got := Adapter{}.TranscriptPath("/Users/a/my.repo", "sid", "/home/h")
	if got != filepath.Join("/home/h", ".claude", "projects", "-Users-a-my-repo", "sid.jsonl") {
		t.Errorf("got %s", got)
	}
}

func TestTrustDialog(t *testing.T) {
	d := Adapter{}.TrustDialog()
	for screen, want := range map[string]bool{
		"Do you TRUST the files in this folder?\n1. Yes\n2. No, exit": true,
		"Do you trust this folder?":                                   false,
		"No, exit":                                                    false,
		"Do you tru\u017ft this?\nNo, exit":                           false,
	} {
		if d.Detect(screen) != want {
			t.Errorf("%q: want %v", screen, want)
		}
	}
	if !reflect.DeepEqual(d.ConfirmKeys, []string{"\x1b[B", "\r"}) {
		t.Errorf("keys %q", d.ConfirmKeys)
	}
	if (Adapter{}).ID() != "claude" || (Adapter{}).Readiness() != "tui-idle" {
		t.Error("id or readiness")
	}
}

func TestArgvAgents(t *testing.T) {
	in := harness.SpecInput{Model: "m", Effort: "low", SessionID: "sid", MCPConfigPath: "/l/mcp.json", LaunchDir: "/l",
		Subagents: []harness.Subagent{
			{Name: "plan", Description: "Plans <a> & b.", Prompt: "You plan.", Model: "claude-haiku-5-5", Tools: []string{"Read"}, ToolsSet: true},
			{Name: "code", Description: "Codes.", Prompt: "You code."},
			{Name: "none", Description: "d", Prompt: "p", ToolsSet: true},
		}}
	got, err := Argv(in)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"claude", "--dangerously-skip-permissions", "--setting-sources", "project", "--settings", filepath.Join("/l", "settings.json"),
		"--strict-mcp-config", "--mcp-config", "/l/mcp.json", "--model", "m", "--effort", "low", "--session-id", "sid",
		"--agents", `{"plan":{"description":"Plans <a> & b.","prompt":"You plan.","model":"claude-haiku-5-5","tools":["Read"]},` +
			`"code":{"description":"Codes.","prompt":"You code."},"none":{"description":"d","prompt":"p","tools":[]}}`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestSpecEnv(t *testing.T) {
	tests := []struct {
		agentsMD string
		want     []harness.EnvVar
	}{
		{"exclude", []harness.EnvVar{{Name: "CLAUDE_CODE_DISABLE_CLAUDE_MDS", Value: "1"}}},
		{"full", nil},
		{"", nil},
	}
	for _, tt := range tests {
		spec, err := Adapter{}.Spec(harness.SpecInput{AgentsMD: tt.agentsMD})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(spec.Env, tt.want) {
			t.Errorf("agents_md %q: env %v, want %v", tt.agentsMD, spec.Env, tt.want)
		}
	}
	if _, err := (Adapter{}).Spec(harness.SpecInput{MCPNames: []string{"missing"}}); err == nil {
		t.Error("an unknown MCP server must fail the spec")
	}
}

func TestArgvPackageDir(t *testing.T) {
	in := harness.SpecInput{Model: "m", Effort: "low", SessionID: "sid", MCPConfigPath: "/l/mcp.json", LaunchDir: "/l", PackageDir: "/l/pkg", InitialPrompt: "Read it."}
	want := []string{"claude", "--dangerously-skip-permissions", "--setting-sources", "project", "--settings", filepath.Join("/l", "settings.json"),
		"--strict-mcp-config", "--mcp-config", "/l/mcp.json", "--add-dir", "/l/pkg", "--model", "m", "--effort", "low",
		"--session-id", "sid", "Read it."}
	got, err := Argv(in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestSettings(t *testing.T) {
	own := "{\n  \"skipDangerousModePermissionPrompt\": true,\n  \"promptSuggestionEnabled\": false"
	got, err := Settings("", nil)
	if err != nil || string(got) != own+"\n}\n" {
		t.Errorf("nothing else: got %s, %v", got, err)
	}
	owner := map[string]any{"theme": "dark", "skipDangerousModePermissionPrompt": false, "env": map[string]any{"A": "1"}}
	got, _ = Settings("", owner)
	if want := own + ",\n  \"env\": {\n    \"A\": \"1\"\n  },\n  \"theme\": \"dark\"\n}\n"; string(got) != want {
		t.Errorf("owner keys: got %s\nwant %s", got, want)
	}
	// The guard groups follow the owner's, in the owner's events and in new ones.
	hooks := map[string]any{"PreToolUse": []any{map[string]any{"matcher": "Edit"}}, "Stop": []any{map[string]any{"hooks": []any{}}}}
	got, err = Settings("'/b/jitso' hook guard", map[string]any{"hooks": hooks})
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Hooks map[string][]map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatal(err)
	}
	pre := parsed.Hooks["PreToolUse"]
	if len(pre) != 3 || pre[0]["matcher"] != "Edit" || pre[1]["matcher"] != "Bash" || pre[2]["matcher"] != "mcp__.*orca.*__.*" {
		t.Errorf("PreToolUse: %v", pre)
	}
	if len(parsed.Hooks["SessionEnd"]) != 1 || len(parsed.Hooks["Stop"]) != 1 {
		t.Errorf("SessionEnd and Stop: %v", parsed.Hooks)
	}
	if len(hooks["PreToolUse"].([]any)) != 1 {
		t.Error("the owner's hooks must not be changed in place")
	}
	if !strings.Contains(string(got), "'/b/jitso' hook guard") {
		t.Errorf("the guard command is missing: %s", got)
	}
}

func TestSpecWritesTheSettingsFile(t *testing.T) {
	in := harness.SpecInput{LaunchDir: "/l", OwnerSettings: harness.OwnerSettings{Doc: map[string]any{"env": map[string]any{"TOKEN": "x"}}}}
	spec, err := Adapter{}.Spec(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Files) != 1 || spec.Files[0].Name != "settings.json" || spec.Files[0].Mode != 0o600 || !strings.Contains(string(spec.Files[0].Data), "TOKEN") {
		t.Errorf("files: %+v", spec.Files)
	}
	if i := slices.Index(spec.Argv, "--settings"); i < 0 || spec.Argv[i+1] != filepath.Join("/l", "settings.json") {
		t.Errorf("argv: %q", spec.Argv)
	}
	for _, word := range spec.Argv {
		if strings.Contains(word, "TOKEN") {
			t.Errorf("a value reached the argv: %q", word)
		}
	}
}
