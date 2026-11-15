package adopt

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/fail"
)

func TestNoop(t *testing.T) {
	tests := []struct {
		name string
		c    Change
		want bool
	}{
		{"same bytes", Change{Kind: KindFile, Old: []byte("a"), New: []byte("a")}, true},
		{"other bytes", Change{Kind: KindFile, Old: []byte("a"), New: []byte("b")}, false},
		{"absent to empty is a creation", Change{Kind: KindFile, New: []byte{}}, false},
		{"empty to nil new is the same", Change{Kind: KindFile, Old: []byte{}}, true},
		{"same link", Change{Kind: KindSymlink, Target: "t", OldTarget: "t"}, true},
		{"other link", Change{Kind: KindSymlink, Target: "u", OldTarget: "t"}, false},
		{"unlink of nothing", Change{Kind: KindSymlink}, true},
		{"unlink", Change{Kind: KindSymlink, OldTarget: "t"}, false},
		{"delete", Change{Kind: KindDelete}, false},
		{"mkdir", Change{Kind: KindMkdir}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.Noop(); got != tt.want {
				t.Errorf("Noop = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlanEmptyAndEffective(t *testing.T) {
	var p Plan
	if !p.Empty() || len(p.Effective()) != 0 {
		t.Error("the zero plan is empty")
	}
	p.Add(Change{Path: "/a", Kind: KindFile, Old: []byte("x"), New: []byte("x")})
	if !p.Empty() {
		t.Error("a plan of no-ops is empty")
	}
	p.Add(Change{Path: "/b", Kind: KindFile, New: []byte("y")})
	if p.Empty() || len(p.Effective()) != 1 || p.Effective()[0].Path != "/b" {
		t.Errorf("effective = %+v", p.Effective())
	}
}

func TestOrderedPutsDeepDeletesFirst(t *testing.T) {
	p := Plan{Changes: []Change{
		{Path: "/r/a", Kind: KindDelete},
		{Path: "/r/x", Kind: KindFile},
		{Path: "/r/a/b/f", Kind: KindDelete},
		{Path: "/r/l", Kind: KindSymlink},
		{Path: "/r/a/b", Kind: KindDelete},
	}}
	got := p.Ordered()
	var paths []string
	for _, c := range got.Changes {
		paths = append(paths, c.Path)
	}
	want := "/r/a/b/f /r/x /r/a/b /r/l /r/a"
	if strings.Join(paths, " ") != want {
		t.Errorf("order = %s, want %s", strings.Join(paths, " "), want)
	}
	if p.Changes[0].Path != "/r/a" {
		t.Error("Ordered changed the receiver")
	}
}

func TestValidate(t *testing.T) {
	root := t.TempDir()
	in := func(rel string) Change {
		return Change{Path: filepath.Join(root, rel), Kind: KindFile}
	}
	tests := []struct {
		name string
		p    Plan
		want string
	}{
		{"ok", Plan{Changes: []Change{in("a"), in("b/c")}}, ""},
		{"relative", Plan{Changes: []Change{{Path: "a", Kind: KindFile}}}, "relative path"},
		{"outside", Plan{Changes: []Change{{Path: filepath.Join(filepath.Dir(root), "x"), Kind: KindFile}}}, "not inside"},
		{"the root itself", Plan{Changes: []Change{{Path: root, Kind: KindFile}}}, "not inside"},
		{"twice", Plan{Changes: []Change{in("a"), in("a")}}, "twice"},
		{"unknown kind", Plan{Changes: []Change{{Path: filepath.Join(root, "a")}}}, "unknown change kind"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.p.Validate(root)
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) || fail.Code(err) != 3 {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestPruneDoesNotMakeAPlanNonEmptyOrShow(t *testing.T) {
	p := Plan{Changes: []Change{{Path: "/r/a", Display: "a", Kind: KindDelete, Prune: true}}}
	if !p.Empty() {
		t.Error("a plan of prunes only is empty")
	}
	p.Add(Change{Path: "/r/f", Display: "f", Kind: KindDelete, Old: []byte("x\n"), Brief: true})
	if p.Empty() {
		t.Fatal("a real change makes the plan non-empty")
	}
	var b bytes.Buffer
	Preview(&b, p)
	if b.String() != "delete f\n" {
		t.Errorf("preview = %q", b.String())
	}
	if len(p.Effective()) != 2 {
		t.Errorf("Apply must still get the prune: %d", len(p.Effective()))
	}
}
