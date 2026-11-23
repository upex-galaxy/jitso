package codex

import (
	"reflect"
	"testing"

	"github.com/upex-galaxy/jitso/internal/harness"
	"github.com/upex-galaxy/jitso/internal/jsonx"
)

func TestArgv(t *testing.T) {
	servers := harness.MCPConfig{Servers: []harness.MCPServer{{Name: "dbhub", Command: "fixture-dbhub", Args: []string{"--transport", "stdio"}}, {Name: "my.server", Command: "x"}}}
	tests := []struct {
		name string
		in   harness.SpecInput
		mcp  harness.MCPConfig
		want []string
	}{
		{"fleet, no guard, no server", harness.SpecInput{Model: "gpt-5.5", Effort: "low"}, harness.MCPConfig{},
			[]string{"codex", "--dangerously-bypass-approvals-and-sandbox", "-m", "gpt-5.5", "-c", `model_reasoning_effort="low"`}},
		{"guard, exclude, servers, prompt", harness.SpecInput{Model: "gpt-5.5", Effort: "high", AgentsMD: "exclude", GuardCommand: "'/b/jitso' hook guard", InitialPrompt: "Read /b.md"}, servers,
			[]string{"codex", "--dangerously-bypass-approvals-and-sandbox", "--dangerously-bypass-hook-trust",
				"-c", `hooks.PreToolUse=[{matcher="Bash",hooks=[{type="command",command="'/b/jitso' hook guard"}]},{matcher="mcp__.*orca.*__.*",hooks=[{type="command",command="'/b/jitso' hook guard"}]}]`,
				"-c", `hooks.SessionEnd=[{hooks=[{type="command",command="'/b/jitso' hook guard"}]}]`,
				"-m", "gpt-5.5", "-c", `model_reasoning_effort="high"`, "-c", "project_doc_max_bytes=0",
				"-c", `mcp_servers.dbhub={command="fixture-dbhub",args=["--transport","stdio"]}`,
				"-c", `mcp_servers."my.server"={command="x",args=[]}`,
				"Read /b.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Argv(tt.in, tt.mcp)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
	if _, err := Argv(harness.SpecInput{}, harness.MCPConfig{Servers: []harness.MCPServer{{Name: "u", Def: jsonx.Object{}}}}); err == nil {
		t.Error("a user server must not reach codex")
	}
}

func TestInline(t *testing.T) {
	tests := []struct {
		in   any
		want string
	}{
		{"a\"b\\c\nd\x01", `"a\"b\\c\nd\u0001"`},
		{true, "true"},
		{3.5, "3.5"},
		{[]string{"x", "y"}, `["x","y"]`},
		{map[string]any{"b": 1, "a": "z"}, `{a="z",b=1}`},
		{jsonx.Object{{Key: "k k", Value: []any{}}}, `{"k k"=[]}`},
	}
	for _, tt := range tests {
		got, err := Inline(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("Inline(%#v) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
	if _, err := Inline(struct{}{}); err == nil {
		t.Error("an unknown type must fail")
	}
}

func TestTrustDialog(t *testing.T) {
	d := Adapter{}.TrustDialog()
	screen := "  Folder access\n  /p\n  Trust this folder? Codex can read, edit, and run files here.\n› 1. Trust and continue\n  2. Back to Agent Command Center\n"
	if !d.Detect(screen) || d.Detect("› ready") || !reflect.DeepEqual(d.ConfirmKeys, []string{"\r"}) {
		t.Errorf("dialog %v", d.ConfirmKeys)
	}
}

func TestNoSessionAndNoTranscript(t *testing.T) {
	if id, err := (Adapter{}).NewSessionID(nil); id != "" || err != nil {
		t.Errorf("session %q, %v", id, err)
	}
	if p := (Adapter{}).TranscriptPath("/c", "s", "/h"); p != "" {
		t.Errorf("transcript %q", p)
	}
}
