package opencode

import (
	"bytes"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/harness"
	"github.com/upex-galaxy/jitso/internal/jsonx"
)

func TestSpec(t *testing.T) {
	in := harness.SpecInput{
		Model: "opencode/m", Effort: "high", SessionID: "ses_1", LaunchDir: "/l",
		MCPNames: []string{"dbhub"}, RepoServers: map[string]any{"dbhub": map[string]any{"command": "db", "args": []any{"-x"}}},
		PackageDir: "/l/pkg", GuardBinary: "/b/jitso", InitialPrompt: "Read /b.md",
	}
	spec, err := Adapter{}.Spec(in)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"opencode", "--standalone", "--auto", "--session", "ses_1", "--prompt", "Read /b.md"}; !reflect.DeepEqual(spec.Argv, want) {
		t.Errorf("argv %q", spec.Argv)
	}
	if want := []harness.EnvVar{{Name: EnvConfigDir, Value: "/l/opencode"}}; !reflect.DeepEqual(spec.Env, want) {
		t.Errorf("env %v", spec.Env)
	}
	if len(spec.Files) != 2 || spec.Files[0].Name != ConfigFile || spec.Files[1].Name != GuardFile {
		t.Fatalf("files %v", spec.Files)
	}
	want := "{\n  \"model\": \"opencode/m#high\",\n  \"mcp\": {\n    \"servers\": {\n      \"dbhub\": {\n        \"type\": \"local\",\n        \"command\": [\n          \"db\",\n          \"-x\"\n        ]\n      }\n    }\n  },\n  \"skills\": [\n    \"/l/pkg/.claude/skills\"\n  ]\n}\n"
	if got := string(spec.Files[0].Data); got != want {
		t.Errorf("config\n%s\nwant\n%s", got, want)
	}
	if !strings.Contains(string(spec.Files[1].Data), `"/b/jitso"`) {
		t.Errorf("guard plugin does not run the binary:\n%s", spec.Files[1].Data)
	}
}

func TestFleetWithoutGuardOrServers(t *testing.T) {
	spec, err := Adapter{}.Spec(harness.SpecInput{Model: "opencode/m", Effort: "low", SessionID: "ses_2", LaunchDir: "/l"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"opencode", "--standalone", "--auto", "--session", "ses_2"}; !reflect.DeepEqual(spec.Argv, want) {
		t.Errorf("argv %q", spec.Argv)
	}
	if len(spec.Files) != 1 || string(spec.Files[0].Data) != "{\n  \"model\": \"opencode/m#low\"\n}\n" {
		t.Errorf("files %v", spec.Files)
	}
	if _, err := Config(harness.SpecInput{}, harness.MCPConfig{Servers: []harness.MCPServer{{Name: "u", Def: jsonx.Object{}}}}); err == nil {
		t.Error("a user server must not reach opencode")
	}
}

func TestNewSessionID(t *testing.T) {
	id, err := Adapter{}.NewSessionID(bytes.NewReader(make([]byte, 16)))
	if err != nil || !regexp.MustCompile(`^ses_[0-9a-f]{32}$`).MatchString(id) {
		t.Errorf("id %q, %v", id, err)
	}
	if (Adapter{}).TrustDialog().Detect("Trust this folder?") {
		t.Error("opencode shows no trust dialog")
	}
}
