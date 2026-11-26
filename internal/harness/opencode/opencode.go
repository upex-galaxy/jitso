// Package opencode is the OpenCode adapter (measured on opencode 2.3.2). A
// launch writes its own OpenCode config folder, <launch>/opencode, and points
// OPENCODE_CONFIG_DIR at it: opencode.json holds the model with the effort as
// its variant (<model>#<effort>), the build's MCP servers and the launch
// package's skills folder, and plugins/jitso-guard.js runs the guard. The
// folder takes the place of the owner's ~/.config/opencode, so the owner's
// own servers and plugins stay out; the project's own opencode.json still
// loads. The TUI runs --standalone, so it reads this environment instead of
// the background service's, with --auto, a fixed --session and the first
// prompt as --prompt.
package opencode

import (
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"

	"github.com/upex-galaxy/jitso/internal/guardfiles"
	"github.com/upex-galaxy/jitso/internal/harness"
	"github.com/upex-galaxy/jitso/internal/jsonx"
	"github.com/upex-galaxy/jitso/internal/version"
)

// Adapter is the OpenCode adapter.
type Adapter struct{}

var _ harness.Adapter = Adapter{}

// ID is "opencode".
func (Adapter) ID() string { return "opencode" }

// Readiness is "tui-idle".
func (Adapter) Readiness() string { return "tui-idle" }

// The launch's config folder and the files in it.
const (
	ConfigDir  = "opencode"
	ConfigFile = ConfigDir + "/opencode.json"
	GuardFile  = ConfigDir + "/plugins/jitso-guard.js"
)

// The variables run.sh exports.
const (
	EnvConfigDir = "OPENCODE_CONFIG_DIR"
)

// Support: MCP servers of the project and the launch package's skills
// (the config's skills key). No subagents and no tool allowlist (not
// measured on OpenCode 2), no AGENTS.md switch that keeps the project's
// skills (OPENCODE_DISABLE_PROJECT_CONFIG drops AGENTS.md together with
// .agents/skills and the project config, measured), no user MCP server.
func (Adapter) Support() harness.Support {
	return harness.Support{MCP: true, PackageFlag: "config"}
}

// Spec builds the argv, the environment and the config folder of one launch.
func (Adapter) Spec(in harness.SpecInput) (harness.Spec, error) {
	mcp, err := harness.CopyServers(in.MCPNames, in.RepoServers, in.UserServers)
	if err != nil {
		return harness.Spec{}, err
	}
	config, err := Config(in, mcp)
	if err != nil {
		return harness.Spec{}, err
	}
	files := []harness.File{{Name: ConfigFile, Data: config}}
	if in.GuardBinary != "" {
		files = append(files, harness.File{Name: GuardFile, Data: guardfiles.OpenCode(in.GuardBinary, version.Current(), in.GuardHome)})
	}
	return harness.Spec{
		Argv:      Argv(in),
		Env:       []harness.EnvVar{{Name: EnvConfigDir, Value: filepath.Join(in.LaunchDir, filepath.FromSlash(ConfigDir))}},
		MCPConfig: mcp,
		Files:     files,
	}, nil
}

// Argv is the opencode argv for in, program first.
func Argv(in harness.SpecInput) []string {
	argv := []string{"opencode", "--standalone", "--auto", "--session", in.SessionID}
	if in.InitialPrompt != "" {
		argv = append(argv, "--prompt", in.InitialPrompt)
	}
	return argv
}

// Config is opencode.json of the launch: model, then mcp when the build
// names a server, then skills when a package folder travels.
func Config(in harness.SpecInput, mcp harness.MCPConfig) ([]byte, error) {
	model := in.Model
	if in.Effort != "" {
		model += "#" + in.Effort
	}
	doc := jsonx.Object{{Key: "model", Value: model}}
	if len(mcp.Servers) > 0 {
		servers := jsonx.Object{}
		for _, s := range mcp.Servers {
			if s.Def != nil {
				return nil, fmt.Errorf(`user mcp server "%s" cannot travel to opencode`, s.Name)
			}
			servers = append(servers, jsonx.Field{Key: s.Name, Value: jsonx.Object{
				{Key: "type", Value: "local"},
				{Key: "command", Value: append([]string{s.Command}, s.Args...)},
			}})
		}
		doc = append(doc, jsonx.Field{Key: "mcp", Value: jsonx.Object{{Key: "servers", Value: servers}}})
	}
	if in.PackageDir != "" {
		doc = append(doc, jsonx.Field{Key: "skills", Value: []string{filepath.Join(in.PackageDir, ".claude", "skills")}})
	}
	data, err := jsonx.MarshalIndent(doc, "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// NewSessionID is "ses_" and 32 hex digits read from rand: OpenCode creates
// the session of an id it does not know, and its ids start with "ses".
func (Adapter) NewSessionID(rand io.Reader) (string, error) {
	var b [16]byte
	if _, err := io.ReadFull(rand, b[:]); err != nil {
		return "", err
	}
	return "ses_" + hex.EncodeToString(b[:]), nil
}

// TranscriptPath is "": OpenCode keeps its sessions in a database.
func (Adapter) TranscriptPath(cwd, session, home string) string { return "" }

// TrustDialog: OpenCode shows none.
func (Adapter) TrustDialog() harness.TrustDialog { return harness.NoTrustDialog() }
