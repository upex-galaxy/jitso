package link

import (
	"strconv"
	"strings"
	"time"

	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/jsonx"
	"github.com/upex-galaxy/jitso/internal/orca"
	"github.com/upex-galaxy/jitso/internal/registry"
)

// Usage is the usage line, in every rejection and for --help.
const Usage = "usage: jitso link <from> <to>"

// ParseArgs reads the two references: exactly two words, neither empty nor
// starting with "-". Anything else rejects the call (exit 2).
func ParseArgs(args []string) (from, to string, err error) {
	if len(args) != 2 {
		return "", "", fail.Rejected(Usage)
	}
	for _, a := range args {
		if a == "" || strings.HasPrefix(a, "-") {
			return "", "", fail.Rejected(Usage)
		}
	}
	return args[0], args[1], nil
}

// Orca is the part of the Orca client link calls.
type Orca interface {
	ListTerminals() ([]orca.TerminalInfo, error)
}

// Deps is what Run needs from the outside.
type Deps struct {
	Store registry.Store
	// Orca returns the client, or the error that says why Orca cannot be
	// asked. It is called only when a reference needs Orca.
	Orca func() (Orca, error)
	Now  func() time.Time
}

// Run resolves both references, appends one link event and returns the line for
// stdout. Nothing is written when a reference cannot be resolved.
//
// A launch id or a recorded terminal handle is resolved by the registry alone.
// A session name, a title, or a handle the registry does not hold, needs Orca's
// terminal list, because a name or a title is accepted only when it names one
// session and a tab may show it too. Without Orca the names and titles of the
// registry still resolve, and a reference they do not hold is the error that
// says Orca is missing.
func Run(from, to string, d Deps) (string, error) {
	launches, err := registry.ReadLaunches(d.Store)
	if err != nil {
		return "", err
	}
	dir := NewDirectory(launches, nil)
	a, aExact := dir.Exact(from)
	b, bExact := dir.Exact(to)
	if !aExact || !bExact {
		client, missing := d.Orca()
		if missing == nil {
			terminals, err := client.ListTerminals()
			if err != nil {
				return "", err
			}
			dir = NewDirectory(launches, terminals)
		}
		if a, err = resolve(dir, from, missing); err != nil {
			return "", err
		}
		if b, err = resolve(dir, to, missing); err != nil {
			return "", err
		}
	}
	if a.Key() == b.Key() {
		return "", fail.Rejected(`"` + from + `" and "` + to + `" are the same session`)
	}
	err = d.Store.Append(&registry.LinkEvent{TS: jsonx.Time(d.Now()), From: end(a), To: end(b)})
	if err != nil {
		return "", err
	}
	return "jitso link: " + a.Key() + " -> " + b.Key() + " recorded", nil
}

// resolve is the one session ref names. missing is why Orca was not asked, nil
// when it was.
func resolve(dir *Directory, ref string, missing error) (Session, error) {
	matches := dir.Match(ref)
	switch {
	case len(matches) == 1:
		return matches[0], nil
	case len(matches) > 1:
		keys := make([]string, len(matches))
		for i, s := range matches {
			keys[i] = s.Key()
		}
		return Session{}, fail.Rejected(`"` + ref + `" names ` + strconv.Itoa(len(matches)) + " sessions (" + strings.Join(keys, ", ") + "): use a terminal handle or a launch id")
	case missing != nil:
		return Session{}, missing
	}
	return Session{}, fail.Rejected(`unknown session "` + ref + `": give a launch id, a terminal handle, a session name or a tab title (see jitso fleet)`)
}

func end(s Session) registry.LinkEnd {
	var e registry.LinkEnd
	if s.ID != "" {
		e.ID = &s.ID
	}
	if s.Handle != "" {
		e.Handle = &s.Handle
	}
	return e
}
