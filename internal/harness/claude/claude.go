// Package claude is the Claude Code adapter. It builds the argv as a
// list: bypass, project-only settings, the two dialog suppression keys, a
// strict MCP file, explicit model and effort, a fixed session id and the rules
// file, then the tool list and the first prompt.
//
// Two decided calls: agents_md: exclude adds
// CLAUDE_CODE_DISABLE_CLAUDE_MDS=1 to the environment (S2), and a build's
// subagents reach claude as --agents <json> (S3).
package claude

import (
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/upex-galaxy/jitso/internal/harness"
	"github.com/upex-galaxy/jitso/internal/jsonx"
	"github.com/upex-galaxy/jitso/internal/settingsjson"
)

// settings are the --settings JSON: skipDangerousModePermissionPrompt and
// promptSuggestionEnabled, in that key order.
const settings = `{"skipDangerousModePermissionPrompt":true,"promptSuggestionEnabled":false}`

// Settings is the --settings JSON of a launch: the two keys, then, when the
// launch carries the guard, the hooks object of the guard's three groups.
// Hooks in --settings run under --setting-sources project (measured in M0-3).
func Settings(guardCommand string) (string, error) {
	if guardCommand == "" {
		return settings, nil
	}
	data, err := jsonx.Marshal(jsonx.Object{
		{Key: "skipDangerousModePermissionPrompt", Value: true},
		{Key: "promptSuggestionEnabled", Value: false},
		{Key: "hooks", Value: settingsjson.HookGroups(guardCommand)},
	})
	return string(data), err
}

// Adapter is the Claude Code adapter.
type Adapter struct{}

var _ harness.Adapter = Adapter{}

// ID is "claude".
func (Adapter) ID() string { return "claude" }

// Binary is the program run.sh execs.
func (Adapter) Binary() string { return "claude" }

// Readiness is "tui-idle".
func (Adapter) Readiness() string { return "tui-idle" }

// Support is everything: Claude Code honors every field of a build.
func (Adapter) Support() harness.Support {
	return harness.Support{Subagents: true, Tools: true, AgentsMDExclude: true, MCP: true, UserMCP: true, PackageFlag: "add-dir", ProjectGuard: true}
}

// Spec builds the argv and the strict MCP file of one launch.
func (a Adapter) Spec(in harness.SpecInput) (harness.Spec, error) {
	mcp, err := MCPConfig(in.MCPNames, in.RepoServers, in.UserServers)
	if err != nil {
		return harness.Spec{}, err
	}
	argv, err := Argv(in)
	if err != nil {
		return harness.Spec{}, err
	}
	return harness.Spec{Argv: argv, Env: Env(in), MCPConfig: mcp}, nil
}

// DisableClaudeMDs is the variable that keeps Claude Code from loading its
// instruction files (CLAUDE.md and, measured on 2.1.325, AGENTS.md).
const DisableClaudeMDs = "CLAUDE_CODE_DISABLE_CLAUDE_MDS"

// Env is what run.sh exports before it execs claude: the S2 variable for
// agents_md: exclude, nothing for full.
func Env(in harness.SpecInput) []harness.EnvVar {
	if in.AgentsMD == "exclude" {
		return []harness.EnvVar{{Name: DisableClaudeMDs, Value: "1"}}
	}
	return nil
}

// AgentsJSON is the --agents value: an object with one key per subagent, in
// order, each holding only description, prompt, model and tools. Only these
// validated fields travel: Claude Code also reads mcpServers, hooks and
// permissionMode from a definition, which would slip past
// --strict-mcp-config or run hooks. A subagent without a model or a tool list
// leaves the key out, so claude applies its own default.
func AgentsJSON(subagents []harness.Subagent) (string, error) {
	agents := jsonx.Object{}
	for _, s := range subagents {
		entry := jsonx.Object{{Key: "description", Value: s.Description}, {Key: "prompt", Value: s.Prompt}}
		if s.Model != "" {
			entry = append(entry, jsonx.Field{Key: "model", Value: s.Model})
		}
		if s.ToolsSet {
			tools := s.Tools
			if tools == nil {
				tools = []string{}
			}
			entry = append(entry, jsonx.Field{Key: "tools", Value: tools})
		}
		agents = append(agents, jsonx.Field{Key: s.Name, Value: entry})
	}
	data, err := jsonx.Marshal(agents)
	return string(data), err
}

// Argv is the argv for in, program first. --agents, when the build
// has subagents, goes right after the session id; --tools and the initial
// prompt, when there are any, follow it.
func Argv(in harness.SpecInput) ([]string, error) {
	settings, err := Settings(in.GuardCommand)
	if err != nil {
		return nil, err
	}
	argv := []string{
		"claude",
		"--dangerously-skip-permissions",
		"--setting-sources", "project",
		"--settings", settings,
		"--strict-mcp-config",
		"--mcp-config", in.MCPConfigPath,
	}
	// --add-dir is variadic: the flag that follows it ends its list.
	if in.PackageDir != "" {
		argv = append(argv, "--add-dir", in.PackageDir)
	}
	argv = append(argv,
		"--model", in.Model,
		"--effort", in.Effort,
		"--session-id", in.SessionID,
	)
	if in.Name != "" {
		argv = append(argv, "--name", in.Name)
	}
	if len(in.Subagents) > 0 {
		agents, err := AgentsJSON(in.Subagents)
		if err != nil {
			return nil, err
		}
		argv = append(argv, "--agents", agents)
	}
	// --tools is variadic: as one --tools=<list> word it cannot swallow the
	// prompt that follows.
	if in.ToolsSet {
		argv = append(argv, "--tools="+strings.Join(in.Tools, ","))
	}
	if in.InitialPrompt != "" {
		argv = append(argv, in.InitialPrompt)
	}
	return argv, nil
}

// MCPConfig is harness.CopyServers: the strict MCP file of a launch.
func MCPConfig(names []string, repoServers map[string]any, userServers map[string]jsonx.Object) (harness.MCPConfig, error) {
	return harness.CopyServers(names, repoServers, userServers)
}

// NewSessionID is a random version 4 UUID read from rand.
func (Adapter) NewSessionID(rand io.Reader) (string, error) { return harness.NewUUID(rand) }

var projectDirChars = regexp.MustCompile(`[^A-Za-z0-9]`)

// TranscriptPath is <home>/.claude/projects/<cwd with every character outside A-Za-z0-9 as -/<session>.jsonl.
func (Adapter) TranscriptPath(cwd, session, home string) string {
	return filepath.Join(home, ".claude", "projects", projectDirChars.ReplaceAllString(cwd, "-"), session+".jsonl")
}

// trustWord is /trust/i spelled out: Go (?i) would also match U+017F for s.
var trustWord = regexp.MustCompile(`[Tt][Rr][Uu][Ss][Tt]`)

// TrustDialog detects the screen by the word "trust" (any case) together with
// "No, exit", and confirms with arrow down then Enter.
func (Adapter) TrustDialog() harness.TrustDialog {
	return harness.TrustDialog{
		Detect: func(screen string) bool {
			return trustWord.MatchString(screen) && strings.Contains(screen, "No, exit")
		},
		ConfirmKeys: []string{"\x1b[B", "\r"},
	}
}
