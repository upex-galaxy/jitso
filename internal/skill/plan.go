package skill

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/upex-galaxy/jitso/internal/adopt"
	"github.com/upex-galaxy/jitso/internal/fail"
)

// Where the skill lives, relative to the scope root (the checkout for project
// scope, HOME for user scope). The real files are in the first folder, which
// Codex and OpenCode read; Claude Code reads only the second, so it gets a
// relative link to the first (AGENTS.md section 8 is the same convention).
const (
	RealRel = ".agents/skills/jitso"
	LinkRel = ".claude/skills/jitso"
	// LinkTarget is the target of the link, relative to the link's folder.
	LinkTarget = "../../.agents/skills/jitso"
)

// Options is what Build needs.
type Options struct {
	// Scope is "project" or "user"; it appears in `not installed at <scope> scope`.
	Scope string
	// Root is the absolute scope root. Every path of the plan lies under it.
	Root string
	// Display renders an absolute path for output: relative to the checkout
	// for project scope, ~/... for user scope. nil prints it as is.
	Display func(string) string
	// Version is the jitso version written into a new manifest.
	Version string
	// Remove plans the removal of the copy instead of its installation.
	Remove bool
	// Force replaces or removes a copy that was edited by hand or was not
	// installed by jitso (a backup is made when the plan is applied).
	Force bool
	// Files are the shipped files; nil means Files().
	Files []File
	// Symlink creates a symbolic link; nil means os.Symlink. A failure makes
	// the second managed copy replace the link.
	Symlink func(target, link string) error
}

// Result is a plan and, when it cannot be applied as it stands, why.
type Result struct {
	Plan adopt.Plan
	// Refusal is the reason a hand-edited or unmanaged copy blocks the run,
	// complete and without the JITSO REJECTED prefix; "" when nothing blocks.
	// The plan is then what --force would do, to show before the refusal.
	Refusal string
}

// Build is the plan of `jitso install skill` for one scope. It reads the
// folders (Inspect) and writes nothing.
func Build(o Options) (Result, error) {
	b := &builder{o: o}
	if b.o.Files == nil {
		b.o.Files = Files()
	}
	b.want = NewManifest(o.Version, b.o.Files)
	var err error
	if b.wantBytes, err = b.want.Marshal(); err != nil {
		return Result{}, fail.Envf("could not encode the skill manifest: %v", err)
	}
	b.real = filepath.Join(o.Root, filepath.FromSlash(RealRel))
	b.link = filepath.Join(o.Root, filepath.FromSlash(LinkRel))

	if o.Remove {
		err = b.remove()
	} else {
		err = b.install()
	}
	if err != nil {
		return Result{}, err
	}
	return Result{Plan: b.plan.Ordered(), Refusal: b.refusal}, nil
}

type builder struct {
	o         Options
	want      Manifest
	wantBytes []byte
	real      string
	link      string
	plan      adopt.Plan
	refusal   string
	// touched records which top folders hold something the plan removes, so a
	// removal prunes only folders it emptied.
	touchedAgents, touchedClaude bool
}

func (b *builder) display(path string) string {
	if b.o.Display == nil {
		return path
	}
	return b.o.Display(path)
}

// inspect is Inspect with a NotFolder answer turned into the error.
func (b *builder) inspect(dir string) (Found, error) {
	f, err := Inspect(dir, b.want)
	if err != nil {
		return Found{}, fail.Envf("could not read %s: %v", b.display(dir), unwrap(err))
	}
	if f.State == NotFolder {
		return Found{}, fail.Envf("%s is %s, not a folder, and jitso does not replace it: move it away and run again", b.display(dir), f.Why)
	}
	return f, nil
}

// refuse records the first reason a copy blocks the run.
func (b *builder) refuse(dir, reason string) {
	if b.refusal != "" {
		return
	}
	verb := "replace"
	if b.o.Remove {
		verb = "remove"
	}
	b.refusal = fmt.Sprintf("the jitso skill in %s %s; nothing written; run again with --force to %s it (a backup is kept)", b.display(dir), reason, verb)
}

// guard decides whether a copy in the state f blocks the run, and says why.
func (b *builder) guard(dir string, f Found) {
	if b.o.Force {
		return
	}
	switch f.State {
	case Modified:
		b.refuse(dir, "was edited by hand ("+strings.Join(f.Edited, ", ")+")")
	case Unmanaged:
		b.refuse(dir, "is not a copy jitso installed ("+f.Why+")")
	}
}

func (b *builder) install() error {
	f, err := b.inspect(b.real)
	if err != nil {
		return err
	}
	b.guard(b.real, f)
	b.sync(b.real, f)
	if f.State == Outdated {
		b.plan.Header = append(b.plan.Header, fmt.Sprintf("upgrade the jitso skill from %s to %s", f.Manifest.JitsoVersion, b.o.Version))
	}
	if err := b.installLink(); err != nil {
		return err
	}
	if b.plan.Empty() && b.refusal == "" {
		switch f.State {
		case Current:
			b.plan.Why = fmt.Sprintf("the jitso skill %s is installed and up to date", f.Manifest.JitsoVersion)
		}
	}
	return nil
}

// sync plans the changes that make dir hold the shipped files and the new
// manifest, and nothing else: it rewrites what differs, adds what is missing,
// deletes every other file and prunes the folders left empty.
func (b *builder) sync(dir string, f Found) {
	type item struct {
		rel  string
		data []byte
	}
	var items []item
	desired := map[string]bool{}
	for _, file := range b.o.Files {
		items = append(items, item{file.Path, file.Data})
		desired[file.Path] = true
	}
	items = append(items, item{ManifestName, b.wantBytes})
	desired[ManifestName] = true

	for _, it := range items {
		abs := filepath.Join(dir, filepath.FromSlash(it.rel))
		var old []byte
		if f.Has(it.rel) {
			old, _ = readRegular(abs)
			if old == nil {
				old = []byte{} // a link or unreadable entry: the write path reports it
			}
		}
		if old != nil && string(old) == string(it.data) {
			continue
		}
		if it.rel == ManifestName && f.State == Current {
			// The content is the shipped one: a newer jitso that ships the same
			// text has nothing to upgrade, and the manifest keeps the version
			// that wrote the files.
			continue
		}
		b.plan.Add(adopt.Change{Path: abs, Display: b.display(abs), Old: old, New: it.data, Kind: adopt.KindFile, Brief: old == nil})
	}
	for _, rel := range f.Files {
		if !desired[rel] {
			abs := filepath.Join(dir, filepath.FromSlash(rel))
			b.plan.Add(adopt.Change{Path: abs, Display: b.display(abs), Old: []byte{}, Kind: adopt.KindDelete, Brief: true})
		}
	}
	keep := map[string]bool{}
	for rel := range desired {
		for d := filepath.ToSlash(filepath.Dir(rel)); d != "."; d = filepath.ToSlash(filepath.Dir(d)) {
			keep[d] = true
		}
	}
	for _, rel := range f.Dirs {
		if !keep[rel] {
			abs := filepath.Join(dir, filepath.FromSlash(rel))
			b.plan.Add(adopt.Change{Path: abs, Display: b.display(abs), Kind: adopt.KindDelete, Prune: true})
		}
	}
}

// installLink plans the entry Claude Code reads: a relative link to the real
// folder, or a second managed copy where links cannot be made.
func (b *builder) installLink() error {
	info, err := os.Lstat(b.link)
	switch {
	case errors.Is(err, os.ErrNotExist):
		ok, why := canSymlink(b.o.Symlink)
		if ok {
			b.plan.Add(adopt.Change{Path: b.link, Display: b.display(b.link), Kind: adopt.KindSymlink, Target: LinkTarget})
			return nil
		}
		b.plan.Header = append(b.plan.Header, fmt.Sprintf("warning: this system cannot create symbolic links (%v); %s gets a second copy of the skill", why, b.display(b.link)))
		b.sync(b.link, Found{State: Absent})
		return nil
	case err != nil:
		return fail.Envf("could not look at %s: %v", b.display(b.link), unwrap(err))
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(b.link)
		if err != nil {
			return fail.Envf("could not read the link %s: %v", b.display(b.link), unwrap(err))
		}
		if pointsTo(b.link, target, b.real) {
			return nil
		}
		b.guard(b.link, Found{State: Unmanaged, Why: "it is a link to " + target})
		b.plan.Add(adopt.Change{Path: b.link, Display: b.display(b.link), Kind: adopt.KindSymlink, Target: LinkTarget, OldTarget: target})
		return nil
	case info.IsDir():
		if sameFolder(b.link, b.real) {
			return nil
		}
		f, err := b.inspect(b.link)
		if err != nil {
			return err
		}
		b.guard(b.link, f)
		b.sync(b.link, f)
		return nil
	}
	return fail.Envf("%s is a file, not a folder or a link, and jitso does not replace it: move it away and run again", b.display(b.link))
}

func (b *builder) remove() error {
	f, err := b.inspect(b.real)
	if err != nil {
		return err
	}
	if f.State != Absent {
		b.guard(b.real, f)
		b.removeDir(b.real, f)
		b.touchedAgents = true
	}

	info, err := os.Lstat(b.link)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return fail.Envf("could not look at %s: %v", b.display(b.link), unwrap(err))
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(b.link)
		if err != nil {
			return fail.Envf("could not read the link %s: %v", b.display(b.link), unwrap(err))
		}
		if pointsTo(b.link, target, b.real) {
			b.plan.Add(adopt.Change{Path: b.link, Display: b.display(b.link), Kind: adopt.KindSymlink, OldTarget: target})
			b.touchedClaude = true
		}
	case info.IsDir() && !sameFolder(b.link, b.real):
		c, err := b.inspect(b.link)
		if err != nil {
			return err
		}
		// A copy jitso did not install is not ours to remove.
		if c.State == Current || c.State == Outdated || c.State == Modified {
			b.guard(b.link, c)
			b.removeDir(b.link, c)
			b.touchedClaude = true
		}
	}

	if b.plan.Empty() && b.refusal == "" {
		b.plan.Why = fmt.Sprintf("the jitso skill is not installed at %s scope", b.o.Scope)
		return nil
	}
	var parents []string
	if b.touchedAgents {
		parents = append(parents, ".agents/skills", ".agents")
	}
	if b.touchedClaude {
		parents = append(parents, ".claude/skills", ".claude")
	}
	for _, rel := range parents {
		abs := filepath.Join(b.o.Root, filepath.FromSlash(rel))
		b.plan.Add(adopt.Change{Path: abs, Display: b.display(abs), Kind: adopt.KindDelete, Prune: true})
	}
	return nil
}

// removeDir plans the deletion of everything in dir and then of dir itself.
func (b *builder) removeDir(dir string, f Found) {
	for _, rel := range f.Files {
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		b.plan.Add(adopt.Change{Path: abs, Display: b.display(abs), Old: []byte{}, Kind: adopt.KindDelete, Brief: true})
	}
	for _, rel := range f.Dirs {
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		b.plan.Add(adopt.Change{Path: abs, Display: b.display(abs), Kind: adopt.KindDelete, Prune: true})
	}
	b.plan.Add(adopt.Change{Path: dir, Display: b.display(dir), Kind: adopt.KindDelete, Prune: true})
}

// pointsTo reports whether the link at linkPath, whose target text is target,
// leads to the folder real: by the path alone (the folder may not exist yet),
// or by the real paths when both exist.
func pointsTo(linkPath, target, real string) bool {
	abs := target
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(filepath.Dir(linkPath), filepath.FromSlash(target))
	}
	if filepath.Clean(abs) == filepath.Clean(real) {
		return true
	}
	return sameFolder(abs, real)
}

// sameFolder reports whether two paths resolve to the same existing folder.
func sameFolder(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

// canSymlink tries to make a link in a scratch folder, so a plan can say
// before anything is written whether the link can be made. why is the reason
// it cannot.
func canSymlink(symlink func(target, link string) error) (ok bool, why error) {
	if symlink == nil {
		symlink = os.Symlink
	}
	dir, err := os.MkdirTemp("", "jitso-link-probe-")
	if err != nil {
		return false, unwrap(err)
	}
	defer os.RemoveAll(dir)
	if err := symlink("probe-target", filepath.Join(dir, "link")); err != nil {
		return false, unwrap(err)
	}
	return true, nil
}

// unwrap drops the operation and paths a path or link error repeats.
func unwrap(err error) error {
	var le *os.LinkError
	if errors.As(err, &le) {
		return le.Err
	}
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
