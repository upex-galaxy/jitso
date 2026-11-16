package launch

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/upex-galaxy/jitso/internal/harness"
	"github.com/upex-galaxy/jitso/internal/jsonx"
	"github.com/upex-galaxy/jitso/internal/orca"
	"github.com/upex-galaxy/jitso/internal/rules"
	"github.com/upex-galaxy/jitso/internal/shell"
	"github.com/upex-galaxy/jitso/internal/trust"
)

// RepoMCPFile is the project's own MCP server table. Only command and args of
// the servers a build names are copied out of it, never env.
const RepoMCPFile = ".mcp.json"

// errNoServerTable is the error for a .mcp.json that is JSON null.
var errNoServerTable = errors.New(RepoMCPFile + " is null, not an object with mcpServers")

// folder is what writeFolder wrote and what the terminal is started with.
type folder struct {
	brief     string
	rules     string
	sessionID string
	argv      []string
	title     string
	command   string
}

// writeFolder writes brief.md, core.md, mcp.json, card.txt and run.sh into
// dir, in the oracle's order of failure: the repo MCP table and the named
// servers first, then the hard rules, then the files.
func writeFolder(p prepared, d Deps, dir string) (folder, error) {
	servers, err := repoServers(filepath.Join(d.Root.Primary, RepoMCPFile))
	if err != nil {
		return folder{}, err
	}
	f := folder{brief: filepath.Join(dir, FileBrief)}
	if f.sessionID, err = p.adapter.NewSessionID(d.Rand); err != nil {
		return folder{}, err
	}
	b := p.profile.Build
	in := harness.SpecInput{
		Model:         p.profile.Model,
		Effort:        p.profile.Effort,
		SessionID:     f.sessionID,
		MCPConfigPath: filepath.Join(dir, FileMCP),
		CorePath:      filepath.Join(dir, FileCore),
		MCPNames:      p.mcp,
		RepoServers:   servers,
		Tools:         b.Tools,
		ToolsSet:      b.ToolsSet,
		AgentsMD:      b.AgentsMD,
		Subagents:     subagentsOf(p.profile.Subagents),
	}
	if p.args.Mode == ModeDirect {
		in.InitialPrompt = FirstPrompt(f.brief, p.profile.RouteScope)
	}
	spec, err := p.adapter.Spec(in)
	if err != nil {
		return folder{}, err
	}
	f.argv = spec.Argv

	if f.rules, err = rules.Load(os.DirFS(d.Root.Primary)); err != nil {
		return folder{}, err
	}

	mcpJSON, err := jsonx.MarshalIndent(spec.MCPConfig, "  ")
	if err != nil {
		return folder{}, err
	}
	script, err := shell.RunScript(spec.Env, spec.Argv)
	if err != nil {
		return folder{}, err
	}
	runSh := filepath.Join(dir, FileRunSh)
	writes := []struct {
		path, text string
	}{
		{f.brief, BriefFile(p.card, b.SkillsFirst, p.briefText)},
		{in.CorePath, f.rules},
		{in.MCPConfigPath, string(mcpJSON) + "\n"},
		{filepath.Join(dir, FileCard), p.card + "\n"},
		{runSh, script},
	}
	for _, w := range writes {
		if err := os.WriteFile(w.path, []byte(w.text), 0o644); err != nil {
			return folder{}, err
		}
	}
	if err := os.Chmod(runSh, 0o755); err != nil {
		return folder{}, err
	}

	mark, err := shell.TerminalTitle(p.id)
	if err != nil {
		return folder{}, err
	}
	f.title = mark + " " + b.ID
	if f.command, err = shell.CommandFor(runSh); err != nil {
		return folder{}, err
	}
	return f, nil
}

// repoServers reads the mcpServers table of the project's .mcp.json. The file
// is required even for a build with no servers (TS-ODDITIES L1). A top level
// that is not an object, or no table, is an empty table: a named server then
// fails as unknown.
func repoServers(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, errors.New(RepoMCPFile + " is not valid JSON: " + err.Error())
	}
	if v == nil {
		return nil, errNoServerTable
	}
	top, _ := v.(map[string]any)
	servers, _ := top["mcpServers"].(map[string]any)
	return servers, nil
}

// trustPolicy is the set of folders a launch may run in.
func trustPolicy(d Deps) trust.Policy {
	return trust.Policy{Primary: d.Root.Primary, Extra: d.TrustExtra}
}

// answerTrust handles the trust dialog of the new terminal. The folder Orca
// reports was already checked by trust.Policy.Place, so the dialog is
// confirmed for a trusted folder (TS-ODDITIES L4, defect N-22).
func answerTrust(adapter harness.Adapter, d Deps, policy trust.Policy, term orca.Terminal) (trust.Outcome, error) {
	screen := terminalScreen{orca: d.Orca, handle: term.Handle}
	return trust.Handle(adapter.TrustDialog(), screen, trust.Clock{Now: d.Now, Sleep: d.Sleep}, policy.Trusted(term.WorktreePath))
}

// terminalScreen is an Orca terminal as a trust.Screen.
type terminalScreen struct {
	orca   Orca
	handle string
}

func (s terminalScreen) Read() (string, error)  { return s.orca.ReadScreen(s.handle) }
func (s terminalScreen) Send(text string) error { return s.orca.SendText(s.handle, text) }
