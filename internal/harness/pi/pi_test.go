package pi

import (
	"reflect"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/harness"
)

func TestArgv(t *testing.T) {
	base := harness.SpecInput{Model: "anthropic/claude-haiku-5-5", Effort: "low", SessionID: "sid", LaunchDir: "/l"}
	head := []string{"pi", "--model", "anthropic/claude-haiku-5-5", "--thinking", "low", "--no-mcp", "--session-id", "sid"}
	tests := []struct {
		name string
		edit func(*harness.SpecInput)
		tail []string
	}{
		{"fleet, harness tools", func(*harness.SpecInput) {}, nil},
		{"no tools", func(in *harness.SpecInput) { in.ToolsSet = true }, []string{"--no-tools"}},
		{"everything", func(in *harness.SpecInput) {
			in.AgentsMD, in.Tools, in.ToolsSet = "exclude", []string{"read", "bash"}, true
			in.PackageDir, in.PackageSkills = "/l/pkg", []string{"humanizer", "jitso"}
			in.GuardBinary, in.InitialPrompt = "/b/jitso", "Read /b.md"
		}, []string{"--no-context-files", "--tools", "read,bash",
			"--skill", "/l/pkg/.claude/skills/humanizer", "--skill", "/l/pkg/.claude/skills/jitso",
			"-e", "/l/pi/jitso-guard.js", "Read /b.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := base
			tt.edit(&in)
			if got, want := Argv(in), append(append([]string(nil), head...), tt.tail...); !reflect.DeepEqual(got, want) {
				t.Errorf("got  %q\nwant %q", got, want)
			}
		})
	}
}

func TestSpecWritesTheGuardExtension(t *testing.T) {
	spec, err := Adapter{}.Spec(harness.SpecInput{Model: "m", Effort: "low", LaunchDir: "/l", GuardBinary: "/b/jitso"})
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Files) != 1 || spec.Files[0].Name != GuardFile || !strings.Contains(string(spec.Files[0].Data), `"/b/jitso"`) {
		t.Errorf("files %v", spec.Files)
	}
	if len(spec.MCPConfig.Servers) != 0 || len(spec.Env) != 0 {
		t.Errorf("pi gets no server and no variable: %+v", spec)
	}
}

func TestTrustDialog(t *testing.T) {
	d := Adapter{}.TrustDialog()
	if !d.Detect("Trust project folder?\n/p\n\nThis allows pi to load .pi settings\n> Trust\n  Do not trust") || d.Detect("ready") {
		t.Error("detect")
	}
	if !reflect.DeepEqual(d.ConfirmKeys, []string{"\r"}) {
		t.Errorf("keys %q", d.ConfirmKeys)
	}
}
