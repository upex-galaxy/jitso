// Package fleet is `jitso fleet`: the read-only list of the sessions Orca
// lists, this project first. It joins the launches of the registry with those
// terminals, so a session can find a peer: the launch id, role and type,
// harness, the title (which is also the session name), the Orca handle, the
// parent, the repository and the state. It writes nothing and remembers
// nothing.
//
// The rows come in three groups: the launches of the project in registry order,
// the unlaunched terminals in a checkout of the project, then the other
// unlaunched terminals, each of the last two in Orca's order. A terminal of
// another repository is never hidden.
//
// A row has one of five states:
//
//	live        the launch has no end event and its terminal is listed
//	gone        the launch has no end event and its terminal is not listed
//	            (jitso reap ends it)
//	unknown     the launch has no end event and Orca could not be asked
//	closed      the launch has an end event (shown only with --all)
//	unlaunched  a listed terminal that no launch explains
//
// A terminal is explained by the handle of any launch of the registry, ended
// or not, or by the [jitso:<id>] mark of one in its title. Orca lists every
// terminal it manages, shells included, so an unlaunched row is a terminal to
// look at, not proof of an agent session.
package fleet

import (
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode"

	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/jsonx"
	"github.com/upex-galaxy/jitso/internal/orca"
	"github.com/upex-galaxy/jitso/internal/registry"
)

// Usage is the usage line, in every rejection and for --help.
const Usage = "usage: jitso fleet [--json] [--all]"

// The states of a row.
const (
	Live       = "live"
	Gone       = "gone"
	Unknown    = "unknown"
	Closed     = "closed"
	Unlaunched = "unlaunched"
)

// stateOrder is the order of the counts in the summary line.
var stateOrder = []string{Live, Gone, Unknown, Closed, Unlaunched}

// Options are the flags of the command.
type Options struct {
	// JSON prints one JSON object instead of the table.
	JSON bool
	// All also shows the launches that have an end event.
	All bool
}

// ParseArgs reads the flags: --json and --all, each at most once, nothing
// else. Anything else rejects the call (exit 2).
func ParseArgs(args []string) (Options, error) {
	var o Options
	for _, a := range args {
		switch {
		case a == "--json" && !o.JSON:
			o.JSON = true
		case a == "--all" && !o.All:
			o.All = true
		default:
			return Options{}, fail.Rejected(Usage)
		}
	}
	return o, nil
}

// Orca is the part of the Orca client fleet calls.
type Orca interface {
	ListTerminals() ([]orca.TerminalInfo, error)
}

// Deps is what Run needs from the outside.
type Deps struct {
	Store registry.Store
	// Orca is nil when Orca cannot be asked (it is not installed): the rows
	// then come from the registry alone and a launch without an end event is
	// unknown.
	Orca Orca
	// Checkouts are the checkout folders of the project (InsideAny), asked
	// for only when Orca listed the terminals; nil means there are none.
	Checkouts func() []string
	// RepoName names the repository of a worktree folder (the function
	// RepoName), asked once for each folder of an unlaunched terminal; nil
	// leaves the repository unknown.
	RepoName func(folder string) string
}

// Row is one session. A field that is not known is "".
type Row struct {
	ID      string
	Parent  string
	Role    string
	Type    string
	Build   string
	Harness string
	Mode    string
	State   string
	Handle  string
	Title   string
	// Repo is the repository the launch event records, and for an unlaunched
	// terminal the repository of its worktree folder.
	Repo     string
	Worktree string
	Branch   string
	TS       string
}

// Run reads the registry and, when d.Orca is set, the terminal list, and
// returns the text for stdout.
func Run(o Options, d Deps) (string, error) {
	launches, err := registry.ReadLaunches(d.Store)
	if err != nil {
		return "", err
	}
	var terminals []orca.TerminalInfo
	if d.Orca != nil {
		if terminals, err = d.Orca.ListTerminals(); err != nil {
			return "", err
		}
	}
	rows := Rows(o, launches, terminals, d.Orca != nil)
	if d.Orca != nil {
		arrange(rows, d)
	}
	if o.JSON {
		return JSON(rows, d.Orca != nil)
	}
	return Text(rows), nil
}

// Rows joins launches with terminals: the launches in registry order (the
// closed ones only with o.All), then the terminals no launch explains in
// Orca's order. orcaKnown is false when terminals could not be listed. The
// repository of an unlaunched row is left empty (Run fills it).
func Rows(o Options, launches []registry.Launch, terminals []orca.TerminalInfo, orcaKnown bool) []Row {
	listed := map[string]orca.TerminalInfo{}
	for _, t := range terminals {
		listed[t.Handle] = t
	}
	explained := map[string]bool{}
	var marks []string
	var rows []Row
	for _, l := range launches {
		e := l.Launch
		r := Row{ID: l.ID}
		r.Parent, _ = e.String("parent")
		r.Role, _ = e.String("role")
		r.Type, _ = e.String("type")
		r.Build, _ = e.String("build")
		r.Harness, _ = e.String("harness")
		r.Mode, _ = e.String("mode")
		r.Handle, _ = e.String("orca.terminal")
		r.Title, _ = e.String("orca.title")
		r.TS, _ = e.String("ts")
		r.Repo, _ = e.String("repo")
		if r.Handle != "" {
			explained[r.Handle] = true
		}
		marks = append(marks, "[jitso:"+l.ID+"]")
		t, isListed := listed[r.Handle]
		switch {
		case l.Ended():
			r.State = Closed
		case !orcaKnown:
			r.State = Unknown
		case isListed:
			r.State = Live
		default:
			r.State = Gone
		}
		if isListed && !l.Ended() {
			if t.Title != "" {
				r.Title = t.Title
			}
			r.Worktree, r.Branch = t.Worktree, t.Branch
		}
		if l.Ended() && !o.All {
			continue
		}
		rows = append(rows, r)
	}
	for _, t := range terminals {
		if explained[t.Handle] || hasMark(t.Title, marks) {
			continue
		}
		rows = append(rows, Row{State: Unlaunched, Handle: t.Handle, Title: t.Title, Worktree: t.Worktree, Branch: t.Branch})
	}
	return rows
}

// arrange names the repository of each unlaunched row of Rows and moves the
// unlaunched terminals of the project before the others, keeping the order
// inside each group.
func arrange(rows []Row, d Deps) {
	var checkouts []string
	if d.Checkouts != nil {
		checkouts = d.Checkouts()
	}
	inProject := InsideAny(checkouts)
	repos := map[string]string{}
	// The handles of the unlaunched terminals of the project.
	own := map[string]bool{}
	for i := range rows {
		r := &rows[i]
		if r.State != Unlaunched || r.Worktree == "" {
			continue
		}
		own[r.Handle] = inProject(r.Worktree)
		if d.RepoName == nil {
			continue
		}
		name, asked := repos[r.Worktree]
		if !asked {
			name = d.RepoName(r.Worktree)
			repos[r.Worktree] = name
		}
		r.Repo = name
	}
	group := func(r Row) int {
		switch {
		case r.State != Unlaunched:
			return 0
		case own[r.Handle]:
			return 1
		}
		return 2
	}
	slices.SortStableFunc(rows, func(a, b Row) int { return group(a) - group(b) })
}

func hasMark(title string, marks []string) bool {
	for _, m := range marks {
		if strings.Contains(title, m) {
			return true
		}
	}
	return false
}

// Text is the table and the summary line, without a trailing newline: a
// header, one aligned row per session, then `jitso fleet: <n> <state>, ...`
// for the states that occur, or `jitso fleet: no sessions`.
func Text(rows []Row) string {
	var b strings.Builder
	if len(rows) > 0 {
		w := tabwriter.NewWriter(&b, 0, 8, 2, ' ', 0)
		cells := func(c ...string) {
			for i, s := range c {
				if i > 0 {
					w.Write([]byte{'\t'})
				}
				w.Write([]byte(cell(s)))
			}
			w.Write([]byte{'\n'})
		}
		cells("ID", "ROLE/TYPE", "HARNESS", "STATE", "HANDLE", "PARENT", "REPO", "TITLE")
		for _, r := range rows {
			roleType := ""
			if r.Role != "" || r.Type != "" {
				roleType = r.Role + "/" + r.Type
			}
			cells(r.ID, roleType, r.Harness, r.State, r.Handle, r.Parent, r.Repo, r.Title)
		}
		w.Flush()
	}
	b.WriteString(Summary(rows))
	return b.String()
}

// Summary is the last line of the text form.
func Summary(rows []Row) string {
	count := map[string]int{}
	for _, r := range rows {
		count[r.State]++
	}
	var parts []string
	for _, s := range stateOrder {
		if n := count[s]; n > 0 {
			parts = append(parts, strconv.Itoa(n)+" "+s)
		}
	}
	if len(parts) == 0 {
		return "jitso fleet: no sessions"
	}
	return "jitso fleet: " + strings.Join(parts, ", ")
}

// cell is one table cell: control characters (a title comes from Orca) become
// spaces, and an empty cell is "-".
func cell(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// JSON is the object `{"orca": <bool>, "sessions": [...]}`, indented by two
// spaces, without a trailing newline. A field that is not known is null.
func JSON(rows []Row, orcaKnown bool) (string, error) {
	sessions := make([]any, 0, len(rows))
	for _, r := range rows {
		sessions = append(sessions, jsonx.Object{
			{Key: "id", Value: nullable(r.ID)},
			{Key: "parent", Value: nullable(r.Parent)},
			{Key: "role", Value: nullable(r.Role)},
			{Key: "type", Value: nullable(r.Type)},
			{Key: "build", Value: nullable(r.Build)},
			{Key: "harness", Value: nullable(r.Harness)},
			{Key: "mode", Value: nullable(r.Mode)},
			{Key: "state", Value: r.State},
			{Key: "handle", Value: nullable(r.Handle)},
			{Key: "title", Value: nullable(r.Title)},
			{Key: "repo", Value: nullable(r.Repo)},
			{Key: "worktree", Value: nullable(r.Worktree)},
			{Key: "branch", Value: nullable(r.Branch)},
			{Key: "ts", Value: nullable(r.TS)},
		})
	}
	data, err := jsonx.MarshalIndent(jsonx.Object{
		{Key: "orca", Value: orcaKnown},
		{Key: "sessions", Value: sessions},
	}, "  ")
	return string(data), err
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
