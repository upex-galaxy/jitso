// Package graph is `jitso graph`: the sessions of a project and of Orca as the
// nodes of a graph, with two kinds of edge, and the local web page that draws
// it.
//
// It needs no project: without a registry, or outside a git checkout, the
// snapshot holds what Orca knows and says why no node is of a project.
//
// A node is a session: a launch of the registry or a terminal Orca lists that
// no launch explains, joined by internal/fleet, so a state means here what it
// means in jitso fleet. A launch with an end event is a node only when a
// running launch descends from it, which keeps the hierarchy whole.
//
// A hierarchy edge goes from a parent launch to a child. A synapse joins two
// sessions that talked, from the links recorded with jitso link and from the
// messages of Orca's orchestration inbox. It holds the direction, the count of
// each direction and the last time, and nothing about what was said.
//
// snapshot.go is pure; collect.go reads the registry and Orca; server.go serves
// the page on 127.0.0.1.
package graph

import (
	"slices"
	"strings"
	"time"

	"github.com/upex-galaxy/jitso/internal/fleet"
	"github.com/upex-galaxy/jitso/internal/jsonx"
	"github.com/upex-galaxy/jitso/internal/link"
	"github.com/upex-galaxy/jitso/internal/orca"
	"github.com/upex-galaxy/jitso/internal/registry"
)

// The directions of a synapse.
const (
	OneWay = "one-way"
	TwoWay = "two-way"
)

// Text is a field that is null in JSON when it is not known.
type Text string

// MarshalJSON writes null for "".
func (t Text) MarshalJSON() ([]byte, error) {
	if t == "" {
		return []byte("null"), nil
	}
	return jsonx.Marshal(string(t))
}

// Node is one session.
type Node struct {
	// Key identifies the node in the edges: the launch id, else the handle.
	Key string `json:"key"`
	ID  Text   `json:"id"`
	// Name is the session name: the title the launch gave the session, else
	// the tab title of a terminal no launch explains.
	Name Text `json:"name"`
	// Title is the tab title Orca shows now, unknown when it lists no
	// terminal for the session.
	Title    Text   `json:"title"`
	Handle   Text   `json:"handle"`
	Role     Text   `json:"role"`
	Type     Text   `json:"type"`
	Build    Text   `json:"build"`
	Harness  Text   `json:"harness"`
	Mode     Text   `json:"mode"`
	State    string `json:"state"`
	Listed   bool   `json:"listed"`
	Worktree Text   `json:"worktree"`
	// Folder is the last element of the worktree path, which says where the
	// session lives.
	Folder Text `json:"folder"`
	Branch Text `json:"branch"`
	Parent Text `json:"parent"`
	// Project is true for a launch of this project and for a terminal in one
	// of its checkouts.
	Project bool `json:"project"`
}

// Hierarchy is a parent to child edge, by node key.
type Hierarchy struct {
	Parent string `json:"parent"`
	Child  string `json:"child"`
}

// Synapse is the talk between two nodes. One-way goes From to To. Of a two-way
// synapse From is the node listed first. Forward counts From to To, Back the
// other way.
type Synapse struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Direction string `json:"direction"`
	Forward   int    `json:"forward"`
	Back      int    `json:"back"`
	Last      Text   `json:"last"`
}

// Snapshot is the graph at one moment.
type Snapshot struct {
	// Orca is false when the terminals could not be listed.
	Orca bool `json:"orca"`
	// Notices say, for a person, what is missing from this snapshot.
	Notices []string `json:"notices"`
	// ProjectEmpty says why no node is of a project, unknown when one is.
	ProjectEmpty Text        `json:"project_empty"`
	Nodes        []Node      `json:"nodes"`
	Hierarchy    []Hierarchy `json:"hierarchy"`
	Synapses     []Synapse   `json:"synapses"`
	// Failure is the first Orca failure behind a notice, nil when there is
	// none. It is not part of the JSON.
	Failure error `json:"-"`
}

// JSON is the snapshot indented by two spaces, without a trailing newline.
func (s Snapshot) JSON() (string, error) {
	data, err := jsonx.MarshalIndent(s, "  ")
	return string(data), err
}

// Focusable reports whether handle is the terminal of a node that Orca lists.
func (s Snapshot) Focusable(handle string) bool {
	return handle != "" && slices.ContainsFunc(s.Nodes, func(n Node) bool { return n.Listed && string(n.Handle) == handle })
}

// builder assembles a snapshot: the nodes first, then the talks between them.
type builder struct {
	dir   *link.Directory
	nodes []Node
	// index is the position of a node by its key.
	index map[string]int
	talks map[pair]*talk
}

// pair is two node positions, the lower first.
type pair struct{ lo, hi int }

type talk struct {
	// up counts lo to hi, down hi to lo.
	up, down int
	last     time.Time
}

// newBuilder makes the nodes: the running launches and their ancestors in
// registry order, then the terminals no launch explains in Orca's order.
// orcaKnown is false when the terminals could not be listed. inProject reports
// whether a worktree folder is a checkout of the project.
func newBuilder(launches []registry.Launch, terminals []orca.TerminalInfo, orcaKnown bool, inProject func(string) bool) *builder {
	b := &builder{dir: link.NewDirectory(launches, terminals), index: map[string]int{}, talks: map[pair]*talk{}}

	byID := map[string]registry.Launch{}
	for _, l := range launches {
		byID[l.ID] = l
	}
	shown := map[string]bool{}
	for _, l := range launches {
		if l.Ended() {
			continue
		}
		// The launch and its chain of parents; shown also stops a cycle of a
		// hand-written log.
		for id := l.ID; !shown[id]; {
			next, ok := byID[id]
			if !ok {
				break
			}
			shown[id] = true
			id, _ = next.Launch.String("parent")
		}
	}

	listed := map[string]orca.TerminalInfo{}
	for _, t := range terminals {
		listed[t.Handle] = t
	}
	for _, r := range fleet.Rows(fleet.Options{All: true}, launches, terminals, orcaKnown) {
		if r.ID != "" && !shown[r.ID] {
			continue
		}
		n := Node{
			Key: link.Session{ID: r.ID, Handle: r.Handle}.Key(),
			ID:  Text(r.ID), Handle: Text(r.Handle),
			Role: Text(r.Role), Type: Text(r.Type), Build: Text(r.Build),
			Harness: Text(r.Harness), Mode: Text(r.Mode), State: r.State,
			Parent: Text(r.Parent), Name: Text(r.Title), Project: r.ID != "",
		}
		if r.ID != "" {
			l := byID[r.ID]
			if title, _ := l.Launch.String("orca.title"); title != "" {
				n.Name = Text(title)
			}
		}
		if t, ok := listed[r.Handle]; ok {
			n.Listed = true
			n.Title, n.Worktree, n.Branch = Text(t.Title), Text(t.Worktree), Text(t.Branch)
			n.Folder = Text(folderName(t.Worktree))
			if !n.Project && t.Worktree != "" && inProject != nil {
				n.Project = inProject(t.Worktree)
			}
		}
		if _, dup := b.index[n.Key]; dup || n.Key == "" {
			continue
		}
		b.index[n.Key] = len(b.nodes)
		b.nodes = append(b.nodes, n)
	}
	return b
}

// node is the position of the node of a terminal handle.
func (b *builder) node(handle string) (int, bool) {
	s, ok := b.dir.ByHandle(handle)
	if !ok {
		return 0, false
	}
	i, ok := b.index[s.Key()]
	return i, ok
}

// end is the position of the node of one side of a link: its launch, else the
// session of its terminal.
func (b *builder) end(id, handle string) (int, bool) {
	if i, ok := b.index[id]; ok && id != "" {
		return i, true
	}
	return b.node(handle)
}

// add counts one talk from a node to another at the time of ts. A node talking
// to itself is no synapse.
func (b *builder) add(from, to int, ts string) {
	if from == to {
		return
	}
	p := pair{min(from, to), max(from, to)}
	t := b.talks[p]
	if t == nil {
		t = &talk{}
		b.talks[p] = t
	}
	if from == p.lo {
		t.up++
	} else {
		t.down++
	}
	if at, ok := registry.ParseTime(ts); ok && at.After(t.last) {
		t.last = at
	}
}

func (b *builder) addLinks(links []registry.Link) {
	for _, l := range links {
		from, fromOK := b.end(l.FromID, l.FromHandle)
		to, toOK := b.end(l.ToID, l.ToHandle)
		if fromOK && toOK {
			b.add(from, to, l.TS)
		}
	}
}

func (b *builder) snapshot() Snapshot {
	s := Snapshot{Notices: []string{}, Nodes: []Node{}, Hierarchy: []Hierarchy{}, Synapses: []Synapse{}}
	s.Nodes = append(s.Nodes, b.nodes...)
	for _, n := range b.nodes {
		parent := string(n.Parent)
		if _, ok := b.index[parent]; ok && parent != n.Key && n.ID != "" {
			s.Hierarchy = append(s.Hierarchy, Hierarchy{Parent: parent, Child: n.Key})
		}
	}
	pairs := make([]pair, 0, len(b.talks))
	for p := range b.talks {
		pairs = append(pairs, p)
	}
	slices.SortFunc(pairs, func(x, y pair) int {
		if x.lo != y.lo {
			return x.lo - y.lo
		}
		return x.hi - y.hi
	})
	for _, p := range pairs {
		t := b.talks[p]
		from, to, forward, back := p.lo, p.hi, t.up, t.down
		if forward == 0 {
			from, to, forward, back = to, from, back, forward
		}
		direction := OneWay
		if back > 0 {
			direction = TwoWay
		}
		var last Text
		if !t.last.IsZero() {
			last = Text(jsonx.Time(t.last))
		}
		s.Synapses = append(s.Synapses, Synapse{
			From: b.nodes[from].Key, To: b.nodes[to].Key, Direction: direction,
			Forward: forward, Back: back, Last: last,
		})
	}
	return s
}

// addressKind splits an Orca address into its prefix ("run", "dispatch" or "")
// and the id or handle after it.
func addressKind(address string) (kind, id string) {
	for _, prefix := range []string{"run", "dispatch"} {
		if rest, ok := strings.CutPrefix(address, prefix+":"); ok {
			return prefix, rest
		}
	}
	return "", address
}
