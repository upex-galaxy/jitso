// Package harness is the per-harness adapter interface and the launch spec
// types. Each harness is one package under harness/ implementing Adapter:
// claude (Claude Code), codex, opencode and pi. The launch code talks only to
// this interface, so it never names a harness flag.
package harness

import (
	"fmt"
	"io"
	"strings"

	"github.com/upex-galaxy/jitso/internal/jsconv"
	"github.com/upex-galaxy/jitso/internal/jsonx"
)

// Adapter builds what a harness needs to start one session.
type Adapter interface {
	// ID is the harness name a build's harness field selects, for example "claude".
	ID() string
	// Spec builds the argv (program first), the environment additions and the
	// strict MCP file contents for one launch. It fails on an MCP server name
	// the repo's own server table does not define.
	Spec(SpecInput) (Spec, error)
	// NewSessionID reads a fresh session id from rand, so the transcript can
	// be found later without guessing.
	NewSessionID(rand io.Reader) (string, error)
	// TranscriptPath is where the harness writes the transcript of session
	// when it runs in cwd with the given home.
	TranscriptPath(cwd, session, home string) string
	// Readiness is the signal `orca terminal wait --for` waits on.
	Readiness() string
	// TrustDialog is how the harness shows its trust-this-folder dialog.
	TrustDialog() TrustDialog
	// Support is what the harness can honor. A build that asks for more is
	// rejected by launch and reported by build validate: an adapter never
	// drops a field it cannot honor.
	Support() Support
}

// Support is what one harness can give a session.
type Support struct {
	// Subagents: the session can get the build's subagents.
	Subagents bool
	// Tools: the session's tools can be limited to the build's list.
	Tools bool
	// AgentsMDExclude: agents_md: exclude can keep the instruction files out.
	AgentsMDExclude bool
	// MCP: the session can get MCP servers; false means it gets none, so a
	// build must name none.
	MCP bool
	// UserMCP: a user:<name> server can travel.
	UserMCP bool
	// PackageFlag is how the launch package folder reaches the session, as
	// the package.flag of the launch event: add-dir, skill or config. "" means
	// the harness cannot load skills from it: a user:<name> skill is
	// rejected, and the jitso skill is left to the session's own
	// .agents/skills.
	PackageFlag string
	// ProjectGuard: the harness reads the guard a project registers in
	// .claude/settings.json, so a launch carries the guard only when the
	// project lacks it. False: every launch carries it.
	ProjectGuard bool
	// Note is one line printed on stdout right after the card of every
	// launch of this harness (a dry run too), "" for none.
	Note string
}

// SpecInput is what an adapter needs for one launch. Paths are absolute.
type SpecInput struct {
	Model     string
	Effort    string
	SessionID string
	// Name is the display name of the session, the same text as the Orca
	// terminal title. An adapter passes it only when its harness has a launch
	// flag for a session name (claude and pi do; codex and opencode have none
	// for their interactive session); "" passes nothing.
	Name string
	// MCPConfigPath is the strict MCP file written for this launch.
	MCPConfigPath string
	// MCPNames are the servers the session gets, in order.
	MCPNames []string
	// RepoServers is the mcpServers table of the repo's .mcp.json, as decoded JSON.
	RepoServers map[string]any
	// UserServers are the copied definitions of the user servers among
	// MCPNames, keyed by the name as the build writes it (user:<name>).
	UserServers map[string]jsonx.Object
	// Tools is the build's tool allowlist; ToolsSet false means the harness
	// default, which is not the same as an empty list.
	Tools    []string
	ToolsSet bool
	// AgentsMD is the build's agents_md value: "full" or "exclude".
	AgentsMD string
	// Subagents are the subagent-tier builds the session may call, in order.
	Subagents []Subagent
	// InitialPrompt is the first prompt (direct mode); "" for none (fleet mode).
	InitialPrompt string
	// PackageDir is the folder of what travels with the launch, whose
	// .claude/skills the session loads; "" when nothing travels.
	PackageDir string
	// PackageSkills are the names of the skills written under
	// PackageDir/.claude/skills, in order.
	PackageSkills []string
	// GuardCommand is the guard hook command the session's settings carry,
	// "" when the project registers the guard itself.
	GuardCommand string
	// GuardBinary is the absolute path of the binary GuardCommand runs, for
	// the harnesses that run the guard from a generated plugin file; "" when
	// no guard travels.
	GuardBinary string
	// LaunchDir is the launch folder, .jitso/launches/<id>; Files of the Spec
	// are written under it.
	LaunchDir string
}

// Subagent is one helper a session may call, from a subagent-tier build.
type Subagent struct {
	Name        string
	Description string
	Prompt      string
	// Model is the helper's model, "" for the harness default.
	Model string
	// Tools is the helper's tool allowlist; ToolsSet false means none given.
	Tools    []string
	ToolsSet bool
}

// Spec is what a launch writes and starts.
type Spec struct {
	// Argv is the full command, program first, as a list: never shell text.
	Argv []string
	// Env are variables run.sh exports before it execs Argv, in order.
	Env []EnvVar
	// MCPConfig is the strict MCP file contents.
	MCPConfig MCPConfig
	// Files are extra files the launch writes under its folder before the
	// session starts (a harness config, a guard plugin), in order.
	Files []File
}

// File is one extra file of a launch folder.
type File struct {
	// Name is the path under the launch folder, with / separators.
	Name string
	Data []byte
}

// EnvVar is one exported variable.
type EnvVar struct {
	Name  string
	Value string
}

// MCPServer is one MCP server as the launcher copies it. A project server
// travels by name, command and args only: its env is never copied. A user
// server travels as Def, the definition with variable references only.
type MCPServer struct {
	Name    string
	Command string
	Args    []string
	// Def, when set, is written as the server's whole entry.
	Def jsonx.Object
}

// MCPConfig is the strict MCP file. It marshals as
// {"mcpServers": {"<name>": {"command": ..., "args": [...]}}} with the servers
// in slice order.
type MCPConfig struct {
	Servers []MCPServer
}

// MarshalJSON writes the servers in order, args never null.
func (c MCPConfig) MarshalJSON() ([]byte, error) {
	servers := jsonx.Object{}
	for _, s := range c.Servers {
		if s.Def != nil {
			servers = append(servers, jsonx.Field{Key: s.Name, Value: s.Def})
			continue
		}
		args := s.Args
		if args == nil {
			args = []string{}
		}
		servers = append(servers, jsonx.Field{Key: s.Name, Value: jsonx.Object{{Key: "command", Value: s.Command}, {Key: "args", Value: args}}})
	}
	return jsonx.Object{{Key: "mcpServers", Value: servers}}.MarshalJSON()
}

// TrustDialog is how a harness shows its trust dialog and how to answer yes.
type TrustDialog struct {
	// Detect reports whether the screen text is the dialog.
	Detect func(screen string) bool
	// ConfirmKeys are raw key strings, sent in order, that confirm trust.
	ConfirmKeys []string
}

// CopyServers copies command and args of each named project server from the
// repo's table. env and every other key are dropped on purpose. A name the
// table does not define, or one without a string command, is an error. A
// user:<name> server is written as its copied definition, under its plain
// name.
func CopyServers(names []string, repoServers map[string]any, userServers map[string]jsonx.Object) (MCPConfig, error) {
	cfg := MCPConfig{Servers: []MCPServer{}}
	for _, name := range names {
		if def, ok := userServers[name]; ok {
			_, plain, _ := strings.Cut(name, ":")
			cfg.Servers = append(cfg.Servers, MCPServer{Name: plain, Def: def})
			continue
		}
		server, _ := repoServers[name].(map[string]any)
		command, ok := server["command"].(string)
		if !ok {
			return MCPConfig{}, fmt.Errorf(`MCP server "%s" is not in the repo MCP config`, name)
		}
		args := []string{}
		if list, ok := server["args"].([]any); ok {
			args = jsconv.Strings(list)
		}
		cfg.Servers = append(cfg.Servers, MCPServer{Name: name, Command: command, Args: args})
	}
	return cfg, nil
}

// NoTrustDialog is the dialog of a harness that never shows one.
func NoTrustDialog() TrustDialog {
	return TrustDialog{Detect: func(string) bool { return false }}
}

// NewUUID is a random version 4 UUID read from rand.
func NewUUID(rand io.Reader) (string, error) {
	var b [16]byte
	if _, err := io.ReadFull(rand, b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
