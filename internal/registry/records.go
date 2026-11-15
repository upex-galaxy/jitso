package registry

import "github.com/upex-galaxy/jitso/internal/jsonx"

// Record is an event a writer appends: one of *LaunchEvent, *EndEvent,
// *AnalyzedEvent or *RuleBreakEvent. Append sets V and Event itself.
type Record interface {
	kind() string
	stamp()
}

// Field rules shared by the records, matching what the oracle writes:
// a field the oracle writes as null is a pointer without omitempty (nil is
// null); a list is never null (stamp turns a nil slice into []); only fields
// the oracle leaves out when unset carry omitempty.

// LaunchEvent is the event a launch writes once its terminal is up.
type LaunchEvent struct {
	V        int     `json:"v"`
	Event    string  `json:"event"`
	ID       string  `json:"id"`
	Parent   *string `json:"parent"`
	Root     string  `json:"root"`
	Depth    int     `json:"depth"`
	TS       string  `json:"ts"`
	Repo     string  `json:"repo"`
	Worktree string  `json:"worktree"`
	Cwd      *string `json:"cwd"`
	Mode     string  `json:"mode"`
	Role     string  `json:"role"`
	Type     string  `json:"type"`
	// Build and BuildVersion keep the v1 log keys "preset" and
	// "preset_version": a storage schema is not renamed with the vocabulary.
	Build        string `json:"preset"`
	BuildVersion any    `json:"preset_version"`
	// BuildLayer, BuildFile and BuildSHA256 name the file the session was
	// launched from: its layer (repo, parent or user), its real absolute path
	// and the SHA-256 of its bytes.
	BuildLayer     string   `json:"build_layer"`
	BuildFile      string   `json:"build_file"`
	BuildSHA256    string   `json:"build_sha256"`
	Harness        string   `json:"harness"`
	Model          string   `json:"model"`
	Effort         string   `json:"effort"`
	Permission     string   `json:"permission"`
	Flags          []string `json:"flags"`
	MCP            []string `json:"mcp"`
	MCPExcluded    []string `json:"mcp_excluded"`
	SkillsFirst    []string `json:"skills_first"`
	RouteScope     []string `json:"route_scope"`
	AgentsMD       string   `json:"agents_md"`
	CoreSHA256     string   `json:"core_sha256"`
	BriefPath      string   `json:"brief_path"`
	BriefSHA256    string   `json:"brief_sha256"`
	BriefLanguage  string   `json:"brief_language"`
	SessionID      string   `json:"session_id"`
	TranscriptPath *string  `json:"transcript_path"`
	TrustDialog    string   `json:"trust_dialog"`
	Orca           OrcaRef  `json:"orca"`
	Budget         *Budget  `json:"budget,omitempty"`
}

// OrcaRef is the orca field of a launch event.
type OrcaRef struct {
	Run      *string `json:"run"`
	Task     *string `json:"task"`
	Terminal string  `json:"terminal"`
	Title    string  `json:"title"`
}

// Budget is the budget of the build a launch ran.
type Budget struct {
	Tokens     float64 `json:"tokens"`
	USDListCap float64 `json:"usd_list_cap"`
}

func (*LaunchEvent) kind() string { return KindLaunch }

func (e *LaunchEvent) stamp() {
	e.V, e.Event = 1, KindLaunch
	for _, list := range []*[]string{&e.Flags, &e.MCP, &e.MCPExcluded, &e.SkillsFirst, &e.RouteScope} {
		if *list == nil {
			*list = []string{}
		}
	}
}

// EndEvent marks a launch as ended (jitso close, reap, the SessionEnd hook).
type EndEvent struct {
	V               int     `json:"v"`
	Event           string  `json:"event"`
	ID              string  `json:"id"`
	TS              string  `json:"ts"`
	Source          string  `json:"source"`
	OutcomeReported string  `json:"outcome_reported"`
	FooterPath      *string `json:"footer_path"`
	DurationS       int64   `json:"duration_s"`
}

func (*EndEvent) kind() string { return KindEnd }
func (e *EndEvent) stamp()     { e.V, e.Event = 1, KindEnd }

// AnalyzedEvent is what jitso analyze appends for a launch.
type AnalyzedEvent struct {
	V           int    `json:"v"`
	Event       string `json:"event"`
	ID          string `json:"id"`
	TS          string `json:"ts"`
	Analyzer    string `json:"analyzer"`
	Turns       int64  `json:"turns"`
	Requests    int64  `json:"requests"`
	PeakContext int64  `json:"peak_context"`
	// BudgetPct is left out when the launch has no token budget.
	BudgetPct     *float64    `json:"budget_pct,omitempty"`
	Tokens        TokenCounts `json:"tokens"`
	CostUSDList   float64     `json:"cost_usd_list"`
	PriceTable    string      `json:"price_table"`
	DurationMin   *float64    `json:"duration_min"`
	SubagentFiles int64       `json:"subagent_files"`
	// ByModel maps a model family to its totals, in the analyzer's order.
	ByModel jsonx.Object `json:"by_model"`
	// ToolsUsed maps a tool name to its call count, in the analyzer's order.
	ToolsUsed     jsonx.Object     `json:"tools_used"`
	RuleBreaks    []RuleBreakCount `json:"rule_breaks"`
	Verdict       string           `json:"verdict"`
	VerdictReason string           `json:"verdict_reason"`
}

// TokenCounts are the token totals of an analysis.
type TokenCounts struct {
	Input      int64 `json:"input"`
	CacheRead  int64 `json:"cache_read"`
	CacheWrite int64 `json:"cache_write"`
	Output     int64 `json:"output"`
}

// RuleBreakCount is one entry of an analyzed event's rule_breaks.
type RuleBreakCount struct {
	Kind  string `json:"kind"`
	Count int64  `json:"count"`
}

func (*AnalyzedEvent) kind() string { return KindAnalyzed }

func (e *AnalyzedEvent) stamp() {
	e.V, e.Event = 1, KindAnalyzed
	if e.ByModel == nil {
		e.ByModel = jsonx.Object{}
	}
	if e.ToolsUsed == nil {
		e.ToolsUsed = jsonx.Object{}
	}
	if e.RuleBreaks == nil {
		e.RuleBreaks = []RuleBreakCount{}
	}
}

// RuleBreakEvent records a broken rule; ID is nil for one with no launch
// (an unlogged worker).
type RuleBreakEvent struct {
	V        int     `json:"v"`
	Event    string  `json:"event"`
	ID       *string `json:"id"`
	TS       string  `json:"ts"`
	Kind     string  `json:"kind"`
	Evidence string  `json:"evidence"`
}

func (*RuleBreakEvent) kind() string { return KindRuleBreak }
func (e *RuleBreakEvent) stamp()     { e.V, e.Event = 1, KindRuleBreak }
