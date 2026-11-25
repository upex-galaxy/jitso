package graph

import (
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/upex-galaxy/jitso/internal/orca"
	"github.com/upex-galaxy/jitso/internal/registry"
)

// InboxLimit is how many of the newest Orca messages a snapshot reads.
const InboxLimit = 5000

// OrcaTimeout is how long graph waits for one Orca call.
const OrcaTimeout = 10 * time.Second

// workerRetry is how long the workers of a run are kept before a dispatch they
// do not hold asks Orca again.
const workerRetry = 30 * time.Second

// heartbeat is the message type of a liveness ping, which is not talk.
const heartbeat = "heartbeat"

// A notice is what went wrong, a colon and what that leaves of the view.
const (
	whatNoOrca     = "Orca is not installed"
	whatNoList     = "Orca did not list its terminals"
	whatNoInbox    = "Orca messages could not be read"
	whatUnresolved = "Some Orca message addresses could not be resolved"

	registryOnly   = ": only the registry is shown, and Orca messages are not read."
	nothingToShow  = ": there are no sessions to show."
	linksOnly      = ": only the synapses recorded with jitso link are shown."
	noSynapse      = ": no synapse is shown."
	synapsesMissed = ": some message synapses are missing."
)

// NoticeUnresolved is the notice of an address that did not resolve.
const NoticeUnresolved = whatUnresolved + synapsesMissed

// The reasons of a view with no session of a project.
const (
	NoProjectCheckout = "No project fleet: this folder is not inside a git checkout."
	NoProjectRegistry = "No project fleet: this repository has no JITSO registry (.jitso/launches.jsonl)."
	NoProjectSession  = "No project fleet: no session of this project is open."
)

// NoticeTruncated is the notice of an inbox that filled the limit.
var NoticeTruncated = "Only the newest " + strconv.Itoa(InboxLimit) + " Orca messages are counted."

// Orca is the part of the Orca client graph calls to build a snapshot.
type Orca interface {
	ListTerminals() ([]orca.TerminalInfo, error)
	Messages(limit int) ([]orca.Message, error)
	RunCoordinators() (map[string]string, error)
	RunWorkers(run string) (map[string]string, error)
}

// what is plain, or that Orca was late with the verb when err says so.
func what(plain, verb string, err error) string {
	var failed *orca.Error
	if errors.As(err, &failed) && failed.Code == orca.CodeTimeout {
		return "Orca did not answer in time (" + verb + ")"
	}
	return plain
}

// Collector reads the registry and Orca into snapshots. It remembers the
// workers of the runs it asked about, so a live view asks Orca once. It is not
// safe for concurrent use: Cache calls it one refresh at a time.
type Collector struct {
	// Store is the project's registry, nil outside a git checkout.
	Store registry.Store
	// Registered reports whether the project has a registry, asked on every
	// snapshot; nil means it has.
	Registered func() bool
	// Orca returns the client, or the error that says why Orca cannot be
	// asked.
	Orca func() (Orca, error)
	// Checkouts are the checkout folders of the project, read on every
	// snapshot because worktrees come and go.
	Checkouts func() []string
	Now       func() time.Time

	workers map[string]*runWorkers
}

type runWorkers struct {
	terminals map[string]string
	err       error
	at        time.Time
}

// Snapshot is the graph now. The error is a registry that cannot be read. An
// Orca that is missing or fails is not an error: the snapshot holds what could
// be known, its notices say what is missing and Failure keeps the cause.
func (c *Collector) Snapshot() (Snapshot, error) {
	// Without a registry there is no project: the view is what Orca knows.
	var events []registry.Event
	empty := ""
	switch {
	case c.Store == nil:
		empty = NoProjectCheckout
	case c.Registered != nil && !c.Registered():
		empty = NoProjectRegistry
	default:
		var err error
		if events, err = c.Store.Events(); err != nil {
			return Snapshot{}, err
		}
	}
	project := empty == ""
	launches := registry.Fold(events)
	noList, noInbox := registryOnly, linksOnly
	if !project {
		noList, noInbox = nothingToShow, noSynapse
	}

	var notices []string
	var failure error
	note := func(text string, cause error) {
		if !slices.Contains(notices, text) {
			notices = append(notices, text)
		}
		if failure == nil {
			failure = cause
		}
	}

	var terminals []orca.TerminalInfo
	known := false
	client, missing := c.Orca()
	if missing != nil {
		note(whatNoOrca+noList, missing)
	} else if list, err := client.ListTerminals(); err != nil {
		note(what(whatNoList, "terminal list", err)+noList, err)
	} else {
		terminals, known = list, true
	}

	var checkouts []string
	if project && known && c.Checkouts != nil {
		checkouts = c.Checkouts()
	}
	b := newBuilder(launches, terminals, known, insideAny(checkouts))
	b.addLinks(registry.Links(events))
	if known {
		c.addMessages(b, client, noInbox, note)
	}

	s := b.snapshot()
	s.Orca, s.Failure = known, failure
	s.Notices = append(s.Notices, notices...)
	if project && !slices.ContainsFunc(s.Nodes, func(n Node) bool { return n.Project }) {
		empty = NoProjectSession
	}
	s.ProjectEmpty = Text(empty)
	return s, nil
}

// addMessages counts the inbox messages between nodes. A run:<id> address is
// the run's coordinator and a dispatch:<id> address the worker of the
// dispatch. The workers of a run are asked for only when the other side of a
// message is already a node, so old mail between closed sessions costs nothing.
// After a call that was late, no further one is made for this snapshot, so a
// hung Orca costs one time limit.
func (c *Collector) addMessages(b *builder, client Orca, noInbox string, note func(string, error)) {
	messages, err := client.Messages(InboxLimit)
	if err != nil {
		note(what(whatNoInbox, "orchestration inbox", err)+noInbox, err)
		return
	}
	if len(messages) >= InboxLimit {
		note(NoticeTruncated, nil)
	}
	var coordinators map[string]string
	late := false
	unresolved := func(verb string, err error) {
		text := what(whatUnresolved, verb, err)
		late = late || text != whatUnresolved
		note(text+synapsesMissed, err)
	}
	if slices.ContainsFunc(messages, func(m orca.Message) bool {
		from, _ := addressKind(m.From)
		to, _ := addressKind(m.To)
		return m.Type != heartbeat && (from == "run" || to == "run")
	}) {
		if coordinators, err = client.RunCoordinators(); err != nil {
			unresolved("orchestration run-list", err)
		}
	}
	for _, m := range messages {
		if m.Type == heartbeat {
			continue
		}
		sides := [2]string{m.From, m.To}
		nodes := [2]int{-1, -1}
		reachable := true
		// First the sides that need no further call.
		for i, address := range sides {
			kind, id := addressKind(address)
			if kind == "dispatch" {
				continue
			}
			if kind == "run" {
				id = coordinators[id]
			}
			n, ok := b.node(id)
			if !ok {
				reachable = false
				break
			}
			nodes[i] = n
		}
		for i, address := range sides {
			kind, id := addressKind(address)
			if !reachable || kind != "dispatch" {
				continue
			}
			handle, err := c.dispatchTerminal(client, m.RunID, id, !late)
			if err != nil {
				unresolved("orchestration worker-list", err)
			}
			n, ok := b.node(handle)
			if !ok {
				reachable = false
				continue
			}
			nodes[i] = n
		}
		if reachable {
			b.add(nodes[0], nodes[1], m.CreatedAt)
		}
	}
}

// dispatchTerminal is the terminal of a dispatch of a run, "" when unknown. ask
// false answers from what is remembered alone.
func (c *Collector) dispatchTerminal(client Orca, run, dispatch string, ask bool) (string, error) {
	if run == "" {
		return "", nil
	}
	entry := c.workers[run]
	if ask && (entry == nil || (entry.terminals[dispatch] == "" && c.Now().Sub(entry.at) >= workerRetry)) {
		terminals, err := client.RunWorkers(run)
		if terminals == nil {
			terminals = map[string]string{}
		}
		next := &runWorkers{terminals: terminals, err: err, at: c.Now()}
		// A failed refresh keeps what was known before it.
		if err != nil && entry != nil {
			for id, handle := range entry.terminals {
				if next.terminals[id] == "" {
					next.terminals[id] = handle
				}
			}
		}
		if c.workers == nil {
			c.workers = map[string]*runWorkers{}
		}
		c.workers[run], entry = next, next
	}
	if entry == nil {
		return "", nil
	}
	if handle := entry.terminals[dispatch]; handle != "" {
		return handle, nil
	}
	return "", entry.err
}
