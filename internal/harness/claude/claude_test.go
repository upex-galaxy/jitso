package claude

import (
	"bytes"
	"reflect"
	"regexp"
	"testing"

	"github.com/upex-galaxy/jitso/internal/harness"
	"github.com/upex-galaxy/jitso/internal/jsonx"
)

func TestArgv(t *testing.T) {
	base := harness.SpecInput{Model: "claude-haiku-5-5", Effort: "low", SessionID: "sid", MCPConfigPath: "/l/mcp.json"}
	head := []string{"claude", "--dangerously-skip-permissions", "--setting-sources", "project", "--settings", settings,
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
	cfg, err := MCPConfig([]string{"openapi", "dbhub"}, repo)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := jsonx.MarshalIndent(cfg, "  ")
	want := "{\n  \"mcpServers\": {\n    \"openapi\": {\n      \"command\": \"fixture-openapi\",\n      \"args\": []\n    },\n" +
		"    \"dbhub\": {\n      \"command\": \"fixture-dbhub\",\n      \"args\": [\n        \"--x\",\n        \"1\"\n      ]\n    }\n  }\n}"
	if string(got) != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	empty, _ := MCPConfig(nil, nil)
	if got, _ := jsonx.Marshal(empty); string(got) != `{"mcpServers":{}}` {
		t.Errorf("empty %s", got)
	}
	for _, name := range []string{"missing", "broken"} {
		if _, err := MCPConfig([]string{name}, repo); err == nil || err.Error() != `MCP server "`+name+`" is not in the repo MCP config` {
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
	if got != "/home/h/.claude/projects/-Users-a-my-repo/sid.jsonl" {
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
	in := harness.SpecInput{Model: "m", Effort: "low", SessionID: "sid", MCPConfigPath: "/l/mcp.json",
		Subagents: []harness.Subagent{
			{Name: "plan", Description: "Plans <a> & b.", Prompt: "You plan.", Model: "claude-haiku-5-5", Tools: []string{"Read"}, ToolsSet: true},
			{Name: "code", Description: "Codes.", Prompt: "You code."},
			{Name: "none", Description: "d", Prompt: "p", ToolsSet: true},
		}}
	got, err := Argv(in)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"claude", "--dangerously-skip-permissions", "--setting-sources", "project", "--settings", settings,
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
	in := harness.SpecInput{Model: "m", Effort: "low", SessionID: "sid", MCPConfigPath: "/l/mcp.json", PackageDir: "/l/pkg", InitialPrompt: "Read it."}
	want := []string{"claude", "--dangerously-skip-permissions", "--setting-sources", "project", "--settings", settings,
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
