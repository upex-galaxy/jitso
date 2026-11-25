// Package pi is the Pi adapter (measured on pi 1.6). It builds the argv as
// a list: model and thinking level, --no-mcp (no server at all: Pi reads its
// servers only from ~/.pi/agent/mcp.json and .pi/mcp.json, so a launch cannot
// give it a strict set), a fixed --session-id, --no-context-files for
// agents_md: exclude, the tool list, one --skill per skill of the launch
// package, the guard as an extension (-e <launch>/pi/jitso-guard.js), then the
// first prompt.
package pi

import (
	"io"
	"path/filepath"
	"strings"

	"github.com/upex-galaxy/jitso/internal/guardfiles"
	"github.com/upex-galaxy/jitso/internal/harness"
	"github.com/upex-galaxy/jitso/internal/version"
)

// Adapter is the Pi adapter.
type Adapter struct{}

var _ harness.Adapter = Adapter{}

// ID is "pi".
func (Adapter) ID() string { return "pi" }

// Readiness is "tui-idle".
func (Adapter) Readiness() string { return "tui-idle" }

// GuardFile is the guard extension under the launch folder.
const GuardFile = "pi/jitso-guard.js"

// Support: a tool allowlist, AGENTS.md off and the launch package's skills.
// No subagents, and no MCP server.
func (Adapter) Support() harness.Support {
	return harness.Support{Tools: true, AgentsMDExclude: true, PackageFlag: "skill"}
}

// Spec builds the argv and the guard extension of one launch.
func (Adapter) Spec(in harness.SpecInput) (harness.Spec, error) {
	mcp, err := harness.CopyServers(in.MCPNames, in.RepoServers, in.UserServers)
	if err != nil {
		return harness.Spec{}, err
	}
	spec := harness.Spec{Argv: Argv(in), MCPConfig: mcp}
	if in.GuardBinary != "" {
		spec.Files = []harness.File{{Name: GuardFile, Data: guardfiles.Pi(in.GuardBinary, version.Current())}}
	}
	return spec, nil
}

// Argv is the pi argv for in, program first.
func Argv(in harness.SpecInput) []string {
	argv := []string{"pi", "--model", in.Model, "--thinking", in.Effort, "--no-mcp", "--session-id", in.SessionID}
	if in.Name != "" {
		argv = append(argv, "--name", in.Name)
	}
	if in.AgentsMD == "exclude" {
		argv = append(argv, "--no-context-files")
	}
	if in.ToolsSet {
		if len(in.Tools) == 0 {
			argv = append(argv, "--no-tools")
		} else {
			argv = append(argv, "--tools", strings.Join(in.Tools, ","))
		}
	}
	for _, name := range in.PackageSkills {
		argv = append(argv, "--skill", filepath.Join(in.PackageDir, ".claude", "skills", name))
	}
	if in.GuardBinary != "" {
		argv = append(argv, "-e", filepath.Join(in.LaunchDir, filepath.FromSlash(GuardFile)))
	}
	if in.InitialPrompt != "" {
		argv = append(argv, in.InitialPrompt)
	}
	return argv
}

// NewSessionID is a random version 4 UUID read from rand.
func (Adapter) NewSessionID(rand io.Reader) (string, error) { return harness.NewUUID(rand) }

// TranscriptPath is "": the session file name starts with the time pi
// creates it, which is not known at launch.
func (Adapter) TranscriptPath(cwd, session, home string) string { return "" }

// TrustDialog detects "Trust project folder?", whose first option is Trust,
// and confirms with Enter. Pi shows it only for a folder with project
// resources (.pi settings, extensions or skills, or .agents/skills) and no
// saved decision, and saves the answer in ~/.pi/agent/trust.json.
func (Adapter) TrustDialog() harness.TrustDialog {
	return harness.TrustDialog{
		Detect:      func(screen string) bool { return strings.Contains(screen, "Trust project folder?") },
		ConfirmKeys: []string{"\r"},
	}
}
