package catalog

import "github.com/upex-galaxy/jitso/internal/jsconv"

// Build is one validated file of jitso/builds/. Text fields hold the String() of the
// decoded value, or "" when the key is missing or has no value; Raw keeps the
// decoded file for anything that needs the value as written.
type Build struct {
	// File is the file name under its builds folder, for example haiku-explorer.yaml.
	File string
	// Layer is where the file lives: repo or draft. It is empty only for a
	// Build made by hand (tests), which reads as repo.
	Layer Layer
	// Display is the file as people see it: jitso/builds/<file> for the
	// project's own builds, .jitso/drafts/<file> for a draft.
	Display string
	// Path is the real absolute path of the file.
	Path string
	// SHA256 is the hex SHA-256 of the file's bytes.
	SHA256 string
	// Raw is the decoded file.
	Raw *Map

	ID     string
	Status string
	// Tier is worker, conductor or subagent; worker when the file has none.
	Tier string
	// Version is the decoded version value, as written (usually an int).
	Version any

	Role        string
	Type        string
	WorkKind    string
	Description string
	Harness     string
	Model       string
	Effort      string
	ModeDefault string
	// AgentsMD is "full" or "exclude".
	AgentsMD string
	// Prompt is the system prompt of a subagent-tier build, "" for the others.
	Prompt string

	RouteScope  []string
	SkillsFirst []string
	MCP         []string
	Requires    []string
	// Subagents are the names of the subagent-tier builds this build lists.
	Subagents []string
	// Tools is the tool allowlist; ToolsSet is false when the build has none,
	// which means the harness default (not an empty list).
	Tools    []string
	ToolsSet bool

	Budget Budget
}

// Budget is the build's budget mapping. Set is false when the build has none.
type Budget struct {
	Tokens     float64
	USDListCap float64
	Set        bool
}

// LayerWord is the layer word of the build, repo for a build made by hand.
func (b *Build) LayerWord() Layer {
	if b.Layer == "" {
		return LayerRepo
	}
	return b.Layer
}

// Launchable is true for a build that `jitso launch` may start:
// approved and not subagent tier.
func (b *Build) Launchable() bool {
	return b.Status == "approved" && b.Tier != TierSubagent
}

// buildFrom turns a decoded file that passed ValidateBuild into a Build.
func buildFrom(file string, m *Map) Build {
	text := func(key string) string { return textOf(m, key) }
	b := Build{
		File:        file,
		Raw:         m,
		ID:          text("id"),
		Status:      text("status"),
		Tier:        TierWorker,
		Version:     m.Value("version"),
		Role:        text("role"),
		Type:        text("type"),
		WorkKind:    text("work_kind"),
		Description: text("description"),
		Harness:     text("harness"),
		Model:       text("model"),
		Effort:      text("effort"),
		ModeDefault: text("mode_default"),
		AgentsMD:    text("agents_md"),
		Prompt:      text("prompt"),
		RouteScope:  jsconv.Strings(m.Value("route_scope")),
		SkillsFirst: jsconv.Strings(m.Value("skills_first")),
		MCP:         jsconv.Strings(m.Value("mcp")),
		Requires:    jsconv.Strings(m.Value("requires")),
		Subagents:   jsconv.Strings(m.Value("subagents")),
	}
	if b.Version == jsconv.Undefined {
		b.Version = nil
	}
	if tier, ok := m.Value("tier").(string); ok {
		b.Tier = tier
	}
	if _, ok := m.Get("harness"); !ok {
		b.Harness = DefaultHarness
	}
	if tools, ok := m.Value("tools").([]any); ok {
		b.Tools = jsconv.Strings(tools)
		b.ToolsSet = true
	}
	if budget, ok := m.Value("budget").(*Map); ok {
		b.Budget = Budget{Tokens: jsNumber(budget.Value("tokens")), USDListCap: jsNumber(budget.Value("usd_list_cap")), Set: true}
	}
	return b
}
