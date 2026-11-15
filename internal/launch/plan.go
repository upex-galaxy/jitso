package launch

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"time"

	"github.com/upex-galaxy/jitso/internal/card"
	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/harness"
	"github.com/upex-galaxy/jitso/internal/jsconv"
	"github.com/upex-galaxy/jitso/internal/jsonx"
	"github.com/upex-galaxy/jitso/internal/profile"
	"github.com/upex-galaxy/jitso/internal/registry"
	"github.com/upex-galaxy/jitso/internal/trust"
)

// IdleTimeoutMS is how long a new terminal may take to reach readiness.
const IdleTimeoutMS = 90_000

// DryRunLine is the line a dry run prints after the card.
const DryRunLine = "dry run: nothing was written and Orca was not called"

// The files of a launch folder, .jitso/launches/<id>/.
const (
	FileBrief = "brief.md"
	FileCore  = "core.md"
	FileMCP   = "mcp.json"
	FileRunSh = "run.sh"
	FileCard  = "card.txt"
)

// HardRules are the two project files joined into core.md, in order.
var HardRules = []string{"jitso/hard-rules/core.md", "jitso/hard-rules/style.md"}

// ID is a launch id, L<yyyymmdd>T<hhmmss>-<4 hex>, from the UTC time and two
// random bytes.
func ID(now time.Time, random [2]byte) string {
	return "L" + now.UTC().Format("20060102T150405") + "-" + hex.EncodeToString(random[:])
}

// Card is the card input of a resolved profile.
func Card(id, mode string, p profile.Profile, mcp []string, launchCommand string) card.Input {
	b := p.Build
	raw := func(key string) string { return jsconv.String(b.Raw.Value(key)) }
	return card.Input{
		ID:            id,
		Role:          raw("role"),
		Type:          raw("type"),
		Mode:          mode,
		Preset:        b.ID,
		PresetVersion: raw("version"),
		Model:         p.Model,
		Effort:        p.Effort,
		MCP:           mcp,
		SkillsFirst:   b.SkillsFirst,
		Subagents:     b.Subagents,
		RouteScope:    p.RouteScope,
		LaunchCommand: launchCommand,
		BudgetTokens:  b.Budget.Tokens,
		BudgetUSD:     b.Budget.USDListCap,
	}
}

// BriefFile is the brief.md the session reads: the card, a blank line, the
// skills to load first (when the build lists any) and the brief as written.
func BriefFile(cardText string, skillsFirst []string, briefText string) string {
	skills := ""
	if len(skillsFirst) > 0 {
		skills = "Load first: " + strings.Join(skillsFirst, ", ") + "\n\n"
	}
	return cardText + "\n\n" + skills + briefText
}

// Rules is core.md: the hard-rule files joined with one newline, as read.
func Rules(files []string) string {
	return strings.Join(files, "\n")
}

// FirstPrompt is the first prompt of a direct launch: the pointer to the
// brief, then the route scope line on a line of its own so the router reads
// it (defect S1). A fleet launch has none: the card at the top of brief.md
// carries the scope.
func FirstPrompt(briefPath string, routeScope []string) string {
	return "Read " + briefPath + " and follow it exactly.\n" + card.ScopeLine(routeScope)
}

// MCPAfterExclude is the build's MCP list without the excluded names (exact
// matches; an unknown name changes nothing).
func MCPAfterExclude(mcp, exclude []string) []string {
	out := []string{}
	for _, name := range mcp {
		excluded := false
		for _, x := range exclude {
			if x == name {
				excluded = true
				break
			}
		}
		if !excluded {
			out = append(out, name)
		}
	}
	return out
}

// SHA256 is the hex sha256 of text.
func SHA256(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// Lineage is where a launch sits in its tree.
type Lineage struct {
	// Parent is the parent launch id, nil for a root launch.
	Parent *string
	// Root is the id of the tree's root launch, "" for a root launch.
	Root  string
	Depth int
}

// EventInput is everything the launch event records.
type EventInput struct {
	ID        string
	Args      Args
	Profile   profile.Profile
	MCP       []string
	Lineage   Lineage
	Repo      string
	Harness   string
	Argv      []string
	Prompted  bool
	Rules     string
	BriefPath string
	BriefText string
	SessionID string
	// Cwd is the folder Orca reported for the terminal, "" for none.
	Cwd        string
	Transcript string
	Trust      trust.Outcome
	Terminal   string
	Title      string
	Now        time.Time
}

// Event is the launch event of in. flags is the argv without the program and,
// in direct mode, without the first prompt.
func Event(in EventInput) *registry.LaunchEvent {
	b := in.Profile.Build
	flags := in.Argv[1:]
	if in.Prompted {
		flags = in.Argv[1 : len(in.Argv)-1]
	}
	root := in.Lineage.Root
	if root == "" {
		root = in.ID
	}
	worktree := "primary"
	if in.Args.Worktree != nil {
		worktree = *in.Args.Worktree
	}
	e := &registry.LaunchEvent{
		ID:             in.ID,
		Parent:         in.Lineage.Parent,
		Root:           root,
		Depth:          in.Lineage.Depth,
		TS:             jsonx.Time(in.Now),
		Repo:           in.Repo,
		Worktree:       worktree,
		Cwd:            nonEmpty(in.Cwd),
		Mode:           in.Args.Mode,
		Role:           b.Role,
		Type:           b.Type,
		Preset:         b.ID,
		PresetVersion:  b.Version,
		Harness:        in.Harness,
		Model:          in.Profile.Model,
		Effort:         in.Profile.Effort,
		Permission:     "bypass",
		Flags:          append([]string{}, flags...),
		MCP:            in.MCP,
		MCPExcluded:    append([]string{}, in.Args.MCPExclude...),
		SkillsFirst:    b.SkillsFirst,
		RouteScope:     in.Profile.RouteScope,
		AgentsMD:       b.AgentsMD,
		CoreSHA256:     SHA256(in.Rules),
		BriefPath:      in.BriefPath,
		BriefSHA256:    SHA256(in.BriefText),
		BriefLanguage:  in.Args.BriefLanguage,
		SessionID:      in.SessionID,
		TranscriptPath: nonEmpty(in.Transcript),
		TrustDialog:    string(in.Trust),
		Orca:           registry.OrcaRef{Run: in.Args.Run, Task: in.Args.Task, Terminal: in.Terminal, Title: in.Title},
	}
	if b.Budget.Set {
		e.Budget = &registry.Budget{Tokens: b.Budget.Tokens, USDListCap: b.Budget.USDListCap}
	}
	return e
}

// AllowedBriefDirs are the folders a brief may be read from: the dispatcher's
// briefs folder and the project's .jitso/briefs.
func AllowedBriefDirs(dispatchDir, stateDir string) []string {
	return []string{filepath.Join(dispatchDir, "briefs"), filepath.Join(stateDir, "briefs")}
}

// subagentsOf are the harness entries of the build's subagent-tier builds.
func subagentsOf(builds []*catalog.Build) []harness.Subagent {
	out := make([]harness.Subagent, len(builds))
	for i, s := range builds {
		out[i] = harness.Subagent{Name: s.ID, Description: s.Description, Prompt: s.Prompt, Model: s.Model, Tools: s.Tools, ToolsSet: s.ToolsSet}
	}
	return out
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
