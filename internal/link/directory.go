// Package link is `jitso link`: it records that one session talked to another,
// as one append-only link line of the registry. The native SendMessage of
// Claude Code leaves no trace jitso can read, so the sender records it.
//
// It also holds the Directory, the one answer to "which session does this word
// name", which jitso graph uses to place messages and links on its nodes.
package link

import (
	"strings"

	"github.com/upex-galaxy/jitso/internal/orca"
	"github.com/upex-galaxy/jitso/internal/registry"
)

// Session is one session of the project's registry or of Orca: a launch id, a
// terminal handle or both. A field that is not known is "".
type Session struct {
	ID     string
	Handle string
}

// Key identifies the session: its launch id, else its terminal handle.
func (s Session) Key() string {
	if s.ID != "" {
		return s.ID
	}
	return s.Handle
}

// Directory indexes the sessions by launch id, terminal handle and name.
type Directory struct {
	ids     map[string]Session
	handles map[string]Session
	names   map[string][]Session
}

// NewDirectory joins the launches of the registry with the terminals Orca
// lists (none when Orca was not asked). It matches them the way jitso fleet
// does: a terminal is a launch's by the launch's handle, else by the
// [jitso:<id>] mark in its title; any other terminal is a session of its own.
//
// A session's names are the session name of its launch, the title its launch
// gave it and the title its tab shows now. The session name and the tab of a
// launch that ended are not among them: another session may take the name, and
// something else may run in the tab.
func NewDirectory(launches []registry.Launch, terminals []orca.TerminalInfo) *Directory {
	d := &Directory{ids: map[string]Session{}, handles: map[string]Session{}, names: map[string][]Session{}}
	ended := map[string]bool{}
	for _, l := range launches {
		handle, _ := l.Launch.String("orca.terminal")
		s := Session{ID: l.ID, Handle: handle}
		d.ids[l.ID] = s
		ended[l.ID] = l.Ended()
		if handle == "" {
			continue
		}
		// A running launch owns its handle over an ended one; among equals the
		// later launch does.
		if old, taken := d.handles[handle]; !taken || !l.Ended() || ended[old.ID] {
			d.handles[handle] = s
		}
	}
	for _, l := range launches {
		if name, _ := l.Launch.String("name"); name != "" && !l.Ended() {
			d.name(name, d.ids[l.ID])
		}
		if title, _ := l.Launch.String("orca.title"); title != "" {
			d.name(title, d.ids[l.ID])
		}
	}
	for _, t := range terminals {
		s, known := d.handles[t.Handle]
		if !known {
			s = Session{Handle: t.Handle}
			for _, l := range launches {
				if strings.Contains(t.Title, "[jitso:"+l.ID+"]") {
					s.ID = l.ID
					break
				}
			}
			d.handles[t.Handle] = s
		}
		if t.Title != "" && !ended[s.ID] {
			d.name(t.Title, s)
		}
	}
	return d
}

func (d *Directory) name(name string, s Session) {
	for _, have := range d.names[name] {
		if have.Key() == s.Key() {
			return
		}
	}
	d.names[name] = append(d.names[name], s)
}

// ByHandle is the session of a terminal handle.
func (d *Directory) ByHandle(handle string) (Session, bool) {
	s, ok := d.handles[handle]
	return s, ok
}

// Exact is the session ref names as a launch id or, failing that, as a
// terminal handle: the two forms that cannot be ambiguous.
func (d *Directory) Exact(ref string) (Session, bool) {
	if s, ok := d.ids[ref]; ok {
		return s, true
	}
	return d.ByHandle(ref)
}

// Match is every session ref names: the one of Exact when there is one, else
// each session with that name, the launches first and then the terminals in
// Orca's order.
func (d *Directory) Match(ref string) []Session {
	if s, ok := d.Exact(ref); ok {
		return []Session{s}
	}
	return d.names[ref]
}
