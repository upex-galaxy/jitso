// Package adopt is the shared core of the adoption commands (`install skill`,
// `setup hook`, `setup instructions`): an apply-plan, a unified diff, the
// confirmation rule, backups and the apply step.
//
// It follows the functional core and imperative shell split. The plan, the
// diff and the confirmation rule are pure. Backup and Apply touch the disk,
// and Run ties them together (preview, decide, apply, report). It never
// imports the command layer: every failure is a package fail error, so the
// caller maps it to an exit code once.
package adopt

import (
	"bytes"
	"path/filepath"
	"sort"
	"strings"

	"github.com/upex-galaxy/jitso/internal/fail"
)

// Kind is what a Change does to its path.
type Kind int

// The change kinds.
const (
	// KindFile writes Change.New as the whole content of a file.
	KindFile Kind = iota + 1
	// KindSymlink points a symbolic link at Change.Target; an empty Target
	// removes the link.
	KindSymlink
	// KindDelete removes a file, a link or an empty folder.
	KindDelete
	// KindMkdir creates a folder (and its parents).
	KindMkdir
)

// Change is one planned effect on one path.
type Change struct {
	// Path is the absolute path.
	Path string
	// Display is the path as printed: relative to the checkout for project
	// scope, written ~/... for user scope.
	Display string
	// Old is the file content now. nil means the file does not exist; an
	// empty non-nil slice is an existing empty file (os.ReadFile gives that).
	Old []byte
	// New is the content to write (KindFile). An empty New writes an empty file.
	New  []byte
	Kind Kind
	// Target is the link target of a KindSymlink change; "" removes the link.
	Target string
	// OldTarget is the target of the link that exists now ("" when there is
	// none), for a KindSymlink change.
	OldTarget string
	// Brief prints the change as one line (new file <path> (<n> lines),
	// delete <path>) instead of a diff.
	Brief bool
	// Sensitive marks a file that may hold secrets (.claude/settings.json):
	// its diff prints no unchanged line and no line of the user's file, only
	// the lines jitso wrote (see UnifiedSensitive), and a diff too large to
	// compute prints only a count of changed lines.
	Sensitive bool
	// Written reports whether a line of a Sensitive file (trimmed of its
	// spaces and a trailing comma) is one jitso writes, so it may print. Nil
	// means none beyond lone braces and brackets.
	Written func(line string) bool
	// Prune marks a KindDelete of a folder that is removed only when it is
	// empty by then (the folders left behind by --remove). It is not
	// previewed and does not make a plan non-empty; Apply runs it in order.
	Prune bool
}

// Existed reports whether the file was there before the change.
func (c Change) Existed() bool { return c.Old != nil }

// Noop reports whether the change would leave the path as it is: the same
// bytes in an existing file, or a link that already has the target (or is
// already absent). Plans are built without no-ops; Run skips any that remain.
func (c Change) Noop() bool {
	switch c.Kind {
	case KindFile:
		return c.Old != nil && bytes.Equal(c.Old, c.New)
	case KindSymlink:
		return c.Target == c.OldTarget
	}
	return false
}

// Plan is the ordered list of changes of one command run.
type Plan struct {
	// Changes are applied and previewed in this order.
	Changes []Change
	// Header lines are printed first when the preview is shown (for example
	// "upgrade the jitso skill from v0.1.0 to v0.2.0").
	Header []string
	// Why completes `jitso <cmd>: nothing to do (<why>)` when the plan has no
	// effective change.
	Why string
	// Notes are printed after the applied lines and before the done line.
	Notes []string
}

// Add appends a change.
func (p *Plan) Add(c Change) { p.Changes = append(p.Changes, c) }

// Effective is the changes that do something, in plan order.
func (p Plan) Effective() []Change {
	out := make([]Change, 0, len(p.Changes))
	for _, c := range p.Changes {
		if !c.Noop() {
			out = append(out, c)
		}
	}
	return out
}

// Empty reports whether no change does anything. A Prune change alone does not count.
func (p Plan) Empty() bool {
	for _, c := range p.Changes {
		if !c.Noop() && !c.Prune {
			return false
		}
	}
	return true
}

// Ordered returns a copy of the plan in which the KindDelete changes keep the
// places the deletes had but run deepest path first, so a folder is removed
// after the files inside it. Every other change stays where it was.
func (p Plan) Ordered() Plan {
	out := p
	out.Changes = append([]Change(nil), p.Changes...)
	var slots []int
	var dels []Change
	for i, c := range out.Changes {
		if c.Kind == KindDelete {
			slots = append(slots, i)
			dels = append(dels, c)
		}
	}
	sort.SliceStable(dels, func(i, j int) bool {
		return depth(dels[i].Path) > depth(dels[j].Path)
	})
	for k, i := range slots {
		out.Changes[i] = dels[k]
	}
	return out
}

func depth(path string) int {
	return strings.Count(filepath.ToSlash(filepath.Clean(path)), "/")
}

// Validate checks the plan against the scope root: every path is absolute,
// lies inside root and appears once, and every kind is known. A violation is
// a bug of the command that built the plan, reported as an environment error.
func (p Plan) Validate(root string) error {
	seen := map[string]bool{}
	for _, c := range p.Changes {
		if !filepath.IsAbs(c.Path) {
			return fail.Envf("internal error: the plan has a relative path %q", c.Path)
		}
		if _, err := relInside(root, c.Path); err != nil {
			return err
		}
		if seen[c.Path] {
			return fail.Envf("internal error: the plan changes %s twice", c.Path)
		}
		seen[c.Path] = true
		switch c.Kind {
		case KindFile, KindSymlink, KindDelete, KindMkdir:
		default:
			return fail.Envf("internal error: the plan has an unknown change kind for %s", c.Path)
		}
	}
	return nil
}

// relInside is path relative to root, in slash form; it fails when path is
// root itself or outside it.
func relInside(root, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fail.Envf("internal error: %s is not inside %s", path, root)
	}
	return filepath.ToSlash(rel), nil
}
