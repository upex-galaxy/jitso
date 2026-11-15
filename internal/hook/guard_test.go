package hook

import (
	"strings"
	"testing"
)

func TestGuardVerb(t *testing.T) {
	l := Launcher{IsOwn: isOwnAt("/opt/jitso")}
	bash := func(command any) map[string]any {
		return map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": command}}
	}
	mcp := func(tool string, args any) map[string]any {
		return map[string]any{"tool_name": "mcp__claude_ai_orca-bridge__" + tool, "tool_input": args}
	}
	tests := []struct {
		name  string
		input map[string]any
		want  string
	}{
		{"empty payload", map[string]any{}, ""},
		{"bash ls", bash("ls"), ""},
		{"bash worker-start", bash("orca orchestration worker-start"), "orchestration worker-start"},
		{"bash command not a string", bash(42.0), ""},
		{"bash command list", bash([]any{"orca", "orchestration", "worker-start"}), ""},
		{"bash without tool_input", map[string]any{"tool_name": "Bash"}, ""},
		{"bash with null tool_input", map[string]any{"tool_name": "Bash", "tool_input": nil}, ""},
		{"no tool name", map[string]any{"tool_input": map[string]any{"command": "orca orchestration worker-start"}}, ""},
		{"lower-case bash is another tool", map[string]any{"tool_name": "bash", "tool_input": map[string]any{"command": "orca orchestration worker-start"}}, ""},
		{"start_worker", mcp("start_worker", map[string]any{}), "orca-bridge start_worker"},
		{"other server", map[string]any{"tool_name": "mcp__other__start_worker"}, ""},
		{"plugin server with orca in its name", map[string]any{"tool_name": "mcp__plugin_orca_orca__start_worker"}, "orca-bridge start_worker"},
		{"create_worktree with agent", mcp("create_worktree", map[string]any{"agent": "claude"}), "orca-bridge create_worktree with an agent"},
		{"create_worktree with empty agent", mcp("create_worktree", map[string]any{"agent": ""}), ""},
		{"create_worktree without agent", mcp("create_worktree", map[string]any{"name": "n"}), ""},
		{"create_terminal harness", mcp("create_terminal", map[string]any{"agent": "codex"}), "orca-bridge create_terminal with a harness"},
		{"create_terminal shell", mcp("create_terminal", map[string]any{"agent": "shell"}), ""},
		{"create_terminal without args", mcp("create_terminal", nil), ""},
		{"read tool", mcp("read_terminal", map[string]any{}), ""},
		{"send_instruction", mcp("send_instruction", map[string]any{}), ""},
		{"other tool", map[string]any{"tool_name": "Write", "tool_input": map[string]any{"command": "orca orchestration worker-start"}}, ""},
		{"build save", bash("jitso build save x"), VerbBuildSave},
		{"build save by own path", bash("/opt/jitso build save x --confirm abc"), VerbBuildSave},
		{"build draft", bash("jitso build draft x.yaml"), ""},
		{"build save in a Write is not Bash", map[string]any{"tool_name": "Write", "tool_input": map[string]any{"command": "jitso build save x"}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := l.GuardVerb(tt.input); got != tt.want {
				t.Errorf("GuardVerb = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLaunchWord(t *testing.T) {
	tests := []struct {
		name, pkg, want string
	}{
		{"script declared", `{"scripts":{"jitso":"bun cli/jitso/index.ts"}}`, "bun run jitso launch"},
		{"no script", `{"scripts":{}}`, "jitso launch"},
		{"no scripts", `{}`, "jitso launch"},
		{"script not a string", `{"scripts":{"jitso":5}}`, "jitso launch"},
		{"not json", `nope`, "jitso launch"},
		{"missing file", ``, "jitso launch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LaunchWord([]byte(tt.pkg)); got != tt.want {
				t.Errorf("LaunchWord = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLauncherWord(t *testing.T) {
	if got := LauncherWord([]byte(`{"scripts":{"jitso":"bun cli/jitso/index.ts"}}`)); got != "bun run jitso" {
		t.Errorf("with a script: %q", got)
	}
	if got := LauncherWord(nil); got != "jitso" {
		t.Errorf("without package.json: %q", got)
	}
}

func TestDenialReason(t *testing.T) {
	script := []byte(`{"scripts":{"jitso":"x"}}`)
	tests := []struct {
		name string
		d    Denial
		pkg  []byte
		want string
	}{
		{"save", Denial{Verb: VerbBuildSave, SaveID: "scout"}, nil,
			"JITSO: only the owner saves a draft as a build; ask the owner to run: jitso build save scout (denied: jitso build save)"},
		{"save with a script", Denial{Verb: VerbBuildSave, SaveID: "<id>"}, script,
			"JITSO: only the owner saves a draft as a build; ask the owner to run: bun run jitso build save <id> (denied: jitso build save)"},
		{"launch", Denial{Verb: "agent launch"}, nil, GuardReason("jitso launch", "agent launch")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.Reason(tt.pkg); got != tt.want {
				t.Errorf("Reason = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGuardReason(t *testing.T) {
	got := GuardReason("jitso launch", "agent launch")
	for _, want := range []string{"jitso launch --build <id>", "(denied: agent launch)"} {
		if !strings.Contains(got, want) {
			t.Errorf("reason %q lacks %q", got, want)
		}
	}
}

func TestAnswer(t *testing.T) {
	got := string(Answer(Deny, "why <id> & more"))
	want := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"why <id> & more"}}` + "\n"
	if got != want {
		t.Errorf("Answer = %q, want %q", got, want)
	}
}
