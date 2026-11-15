// Package harness is the per-harness adapter interface and the launch spec
// types. Claude Code is the first adapter (package harness/claude); another
// harness is another package under harness/ implementing Adapter. The launch
// code talks only to this interface, so it never names a harness flag.
package harness

import (
	"io"

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
}

// SpecInput is what an adapter needs for one launch. Paths are absolute.
type SpecInput struct {
	Model     string
	Effort    string
	SessionID string
	// MCPConfigPath is the strict MCP file written for this launch.
	MCPConfigPath string
	// CorePath is the rules file appended to the system prompt.
	CorePath string
	// MCPNames are the servers the session gets, in order.
	MCPNames []string
	// RepoServers is the mcpServers table of the repo's .mcp.json, as decoded JSON.
	RepoServers map[string]any
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
}

// EnvVar is one exported variable.
type EnvVar struct {
	Name  string
	Value string
}

// MCPServer is one MCP server as the launcher copies it: by name, command and
// args only. Its env is never copied: values never travel.
type MCPServer struct {
	Name    string
	Command string
	Args    []string
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
