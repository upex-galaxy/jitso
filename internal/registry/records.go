package registry

import (
	"encoding/json"

	"github.com/upex-galaxy/jitso/internal/usage"
)

// Record is an event a writer appends: one of *LaunchEvent or *EndEvent. Append sets V and Event itself.
type Record interface {
	kind() string
	stamp()
}

// Field rules shared by the records, matching what a launch writes:
// a field written as null is a pointer without omitempty (nil is
// null); a list is never null (stamp turns a nil slice into []); only fields
// left out when unset carry omitempty.

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
	// Build is the id of the build the session was launched from and
	// BuildVersion its version, as the build file spells it.
	Build        string `json:"build"`
	BuildVersion any    `json:"build_version"`
	// BuildLayer, BuildFile and BuildSHA256 name the file the session was
	// launched from: its layer (repo or draft), its real absolute path
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
	BriefPath      string   `json:"brief_path"`
	BriefSHA256    string   `json:"brief_sha256"`
	BriefLanguage  string   `json:"brief_language"`
	SessionID      string   `json:"session_id"`
	TranscriptPath *string  `json:"transcript_path"`
	TrustDialog    string   `json:"trust_dialog"`
	Orca           OrcaRef  `json:"orca"`
	Budget         *Budget  `json:"budget,omitempty"`
	// Fields are the --field values of the launch, key to value, in key
	// order; the key is left out when the launch has none.
	Fields map[string]string `json:"fields,omitempty"`
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
	e.V, e.Event = Version, KindLaunch
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
	// Tokens is set only by jitso close: the key is left out of every other
	// end event, and a close with no readable transcript writes it as null.
	Tokens *Tokens `json:"tokens,omitempty"`
}

// Tokens is the tokens field of a close end event; a nil Totals is JSON null.
type Tokens struct{ Totals *usage.Totals }

// MarshalJSON writes the totals, or null when there are none.
func (t Tokens) MarshalJSON() ([]byte, error) {
	if t.Totals == nil {
		return []byte("null"), nil
	}
	return json.Marshal(t.Totals)
}

func (*EndEvent) kind() string { return KindEnd }
func (e *EndEvent) stamp()     { e.V, e.Event = Version, KindEnd }
