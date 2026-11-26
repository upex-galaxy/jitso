package launch

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/project"
	"github.com/upex-galaxy/jitso/internal/shell"
	"github.com/upex-galaxy/jitso/internal/trust"
)

// Target is the repository a launch sends its worker into. The home
// repository (Deps.Root) keeps the builds, the briefs and the registry.
type Target struct {
	// Root is the repository: its primary checkout resolves skills, MCP
	// servers, route ids and instructions, and Checkout is the checkout the
	// worker runs in.
	project.Root
	// Cross is true when it is not the home repository.
	Cross bool
}

// ResolveRepo is the repository a --repo path is in: the path is checked,
// cleaned, resolved through its symlinks and must lie inside a git checkout.
// Anything else is a rejection.
func ResolveRepo(path string) (project.Root, error) {
	if reason := shell.CheckLaunchField("repo", path); reason != "" {
		return project.Root{}, fail.Rejected(reason)
	}
	r, err := project.Resolve(filepath.Clean(path))
	if err != nil {
		return project.Root{}, fail.Rejected("--repo " + path + " is not inside a git checkout")
	}
	return r, nil
}

// ResolveTarget is the target of a launch from home: the repository of --repo,
// else the one a --worktree path:<folder> belongs to when that is another
// repository, else home. A --worktree that names a path must name a worktree
// of the target; one that does not is a rejection here for --repo and for
// another repository, and in the launch itself otherwise.
func ResolveTarget(a Args, home project.Root) (Target, error) {
	var repo project.Root
	switch {
	case a.Repo != nil:
		r, err := ResolveRepo(*a.Repo)
		if err != nil {
			return Target{}, err
		}
		repo = r
	default:
		// Only the path: form names a target; an id: selector only names a
		// worktree of one.
		selector := deref(a.Worktree)
		path, ok := trust.SelectorPath(selector)
		if !ok || !strings.HasPrefix(selector, "path:") || !filepath.IsAbs(path) {
			return Target{Root: home}, nil
		}
		r, err := project.Resolve(path)
		if err != nil || project.SameDir(r.Primary, home.Primary) {
			return Target{Root: home}, nil
		}
		repo = r
	}
	t := Target{Root: project.Root{Primary: repo.Primary, Checkout: repo.Primary}, Cross: !project.SameDir(repo.Primary, home.Primary)}
	if !t.Cross {
		t.Primary, t.Checkout = home.Primary, home.Primary
	}
	if a.Worktree != nil {
		if why := policyOf(t, nil).Precheck(*a.Worktree); why != "" {
			return Target{}, fail.Rejected(why)
		}
		if path, ok := trust.SelectorPath(*a.Worktree); ok {
			t.Checkout = project.Real(path)
		}
	}
	return t, nil
}

// policyOf is the set of folders a launch into t may run in.
func policyOf(t Target, extra []string) trust.Policy {
	p := trust.Policy{Primary: t.Primary, Extra: extra}
	if t.Cross {
		p.Repo = t.Primary
	}
	return p
}

// openSelector is the --worktree Orca is given: the one of the command line,
// else path:<target> when --repo, or a JITSO_PROJECT that names another
// folder than the caller's, says the terminal does not belong where Orca
// would open it by itself. "" leaves the flag out.
func openSelector(a Args, home project.Root, t Target) string {
	switch {
	case a.Worktree != nil:
		return *a.Worktree
	case a.Repo != nil || home.Away:
		return "path:" + filepath.ToSlash(t.Primary)
	}
	return ""
}

// registered reports whether Orca has the repository of primary registered:
// one of the folders it lists is the primary checkout or a worktree of it. The
// error is a repository list that cannot be read.
func registered(o Orca, primary string) (bool, error) {
	paths, err := o.RepoPaths()
	if err != nil {
		return false, err
	}
	checkouts := project.Checkouts(project.Root{Primary: primary})
	return slices.ContainsFunc(paths, func(path string) bool {
		return slices.ContainsFunc(checkouts, func(c string) bool { return project.SameDir(path, c) })
	}), nil
}

var plainPath = regexp.MustCompile(`^[A-Za-z0-9_./:\\-]+$`)

// unregistered is the refusal of a target Orca has not registered, with the
// line that registers it. JITSO never runs that line itself.
func unregistered(primary string) error {
	word := primary
	if !plainPath.MatchString(word) {
		quoted, err := shell.Quote(word)
		if err != nil {
			return err
		}
		word = quoted
	}
	return fail.Envf("%s is not registered in Orca, so Orca opens no terminal there. Register it with: orca repo add --path %s", primary, word)
}
