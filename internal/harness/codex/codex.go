// Package codex is the Codex CLI adapter (measured on codex-cli 0.171). It
// builds the argv as a list: bypass of approvals and sandbox, the guard as
// inline hooks (-c hooks.<event>=..., which run only with
// --dangerously-bypass-hook-trust), explicit model and reasoning effort,
// project_doc_max_bytes=0 for agents_md: exclude, one -c mcp_servers.<n> per
// server of the build, then the first prompt.
//
// Codex merges every -c override into the owner's ~/.codex/config.toml, so
// the owner's own Codex MCP servers stay available next to the build's: a
// codex launch says so after its card (Support.Note). Codex picks its own session
// id, so the launch records none and no transcript.
package codex

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/upex-galaxy/jitso/internal/harness"
	"github.com/upex-galaxy/jitso/internal/jsonx"
	"github.com/upex-galaxy/jitso/internal/settingsjson"
)

// Adapter is the Codex CLI adapter.
type Adapter struct{}

var _ harness.Adapter = Adapter{}

// ID is "codex".
func (Adapter) ID() string { return "codex" }

// Readiness is "tui-idle".
func (Adapter) Readiness() string { return "tui-idle" }

// Note is the line printed after the card of every codex launch.
const Note = "jitso: note: codex keeps the MCP servers of the owner's own Codex config; the build's servers are added to them"

// Support: MCP servers of the project, AGENTS.md off; no subagents, no tool
// allowlist, no skills from the launch package (codex reads only
// .agents/skills of the project and of the user), no user MCP server.
func (Adapter) Support() harness.Support {
	return harness.Support{AgentsMDExclude: true, MCP: true, Note: Note}
}

// Spec builds the argv of one launch; the MCP file is written as the
// record of the servers the build named.
func (Adapter) Spec(in harness.SpecInput) (harness.Spec, error) {
	mcp, err := harness.CopyServers(in.MCPNames, in.RepoServers, in.UserServers)
	if err != nil {
		return harness.Spec{}, err
	}
	argv, err := Argv(in, mcp)
	if err != nil {
		return harness.Spec{}, err
	}
	return harness.Spec{Argv: argv, MCPConfig: mcp}, nil
}

// Argv is the codex argv for in, program first.
func Argv(in harness.SpecInput, mcp harness.MCPConfig) ([]string, error) {
	argv := []string{"codex", "--dangerously-bypass-approvals-and-sandbox"}
	if in.GuardCommand != "" {
		argv = append(argv, "--dangerously-bypass-hook-trust")
		for _, event := range settingsjson.HookGroups(in.GuardCommand) {
			value, err := Inline(event.Value)
			if err != nil {
				return nil, err
			}
			argv = append(argv, "-c", "hooks."+Key(event.Key)+"="+value)
		}
	}
	argv = append(argv, "-m", in.Model, "-c", "model_reasoning_effort="+String(in.Effort))
	if in.AgentsMD == "exclude" {
		argv = append(argv, "-c", "project_doc_max_bytes=0")
	}
	for _, s := range mcp.Servers {
		if s.Def != nil {
			return nil, fmt.Errorf(`user mcp server "%s" cannot travel to codex`, s.Name)
		}
		args := make([]any, len(s.Args))
		for i, a := range s.Args {
			args[i] = a
		}
		value, err := Inline(jsonx.Object{{Key: "command", Value: s.Command}, {Key: "args", Value: args}})
		if err != nil {
			return nil, err
		}
		argv = append(argv, "-c", "mcp_servers."+Key(s.Name)+"="+value)
	}
	if in.InitialPrompt != "" {
		argv = append(argv, in.InitialPrompt)
	}
	return argv, nil
}

var bareKey = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Key is k as a TOML key: bare when it may be, else a basic string.
func Key(k string) string {
	if bareKey.MatchString(k) {
		return k
	}
	return String(k)
}

// String is s as a TOML basic string.
func String(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Inline is v as a one-line TOML value: an object as an inline table, a list
// as an array, a string, a bool or a number. The value of a -c override.
func Inline(v any) (string, error) {
	switch t := v.(type) {
	case string:
		return String(t), nil
	case bool:
		return strconv.FormatBool(t), nil
	case int:
		return strconv.Itoa(t), nil
	case float64:
		return strconv.FormatFloat(t, 'g', -1, 64), nil
	case jsonx.Object:
		parts := make([]string, len(t))
		for i, f := range t {
			value, err := Inline(f.Value)
			if err != nil {
				return "", err
			}
			parts[i] = Key(f.Key) + "=" + value
		}
		return "{" + strings.Join(parts, ",") + "}", nil
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		obj := make(jsonx.Object, len(keys))
		for i, k := range keys {
			obj[i] = jsonx.Field{Key: k, Value: t[k]}
		}
		return Inline(obj)
	case []any:
		parts := make([]string, len(t))
		for i, item := range t {
			value, err := Inline(item)
			if err != nil {
				return "", err
			}
			parts[i] = value
		}
		return "[" + strings.Join(parts, ",") + "]", nil
	case []string:
		items := make([]any, len(t))
		for i, item := range t {
			items[i] = item
		}
		return Inline(items)
	}
	return "", fmt.Errorf("codex: cannot write %T as TOML", v)
}

// NewSessionID is "": codex picks its own session id and takes none.
func (Adapter) NewSessionID(io.Reader) (string, error) { return "", nil }

// TranscriptPath is "": the rollout file name holds a time and an id codex
// picks, so it is not known at launch.
func (Adapter) TranscriptPath(cwd, session, home string) string { return "" }

// TrustDialog detects "Trust this folder?" with its "Trust and continue"
// option, which is selected first, and confirms with Enter. Codex then saves
// the folder as trusted in ~/.codex/config.toml.
func (Adapter) TrustDialog() harness.TrustDialog {
	return harness.TrustDialog{
		Detect: func(screen string) bool {
			return strings.Contains(screen, "Trust this folder?") && strings.Contains(screen, "Trust and continue")
		},
		ConfirmKeys: []string{"\r"},
	}
}
