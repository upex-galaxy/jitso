package trust

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/upex-galaxy/jitso/internal/harness"
)

var dialog = harness.TrustDialog{
	Detect:      func(s string) bool { return strings.Contains(s, "trust") && strings.Contains(s, "No, exit") },
	ConfirmKeys: []string{"\x1b[B", "\r"},
}

const dialogScreen = "Do you trust this folder?\n2. No, exit"

// fakeScreen answers reads from screens (the last repeats) and records sends.
type fakeScreen struct {
	screens []string
	reads   int
	sent    []string
	readErr error
	sendErr error
}

func (f *fakeScreen) Read() (string, error) {
	if f.readErr != nil {
		return "", f.readErr
	}
	s := f.screens[min(f.reads, len(f.screens)-1)]
	f.reads++
	return s, nil
}

func (f *fakeScreen) Send(text string) error {
	f.sent = append(f.sent, text)
	return f.sendErr
}

// fakeClock advances only when Sleep is called.
func fakeClock() Clock {
	now := time.Unix(0, 0)
	return Clock{Now: func() time.Time { return now }, Sleep: func(d time.Duration) { now = now.Add(d) }}
}

func TestHandle(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name      string
		screen    *fakeScreen
		trusted   bool
		want      Outcome
		wantErr   bool
		wantSent  []string
		wantReads int
	}{
		{"absent", &fakeScreen{screens: []string{"❯ idle"}}, true, Absent, false, nil, 1},
		{"refused", &fakeScreen{screens: []string{dialogScreen}}, false, Refused, false, nil, 1},
		{"handled", &fakeScreen{screens: []string{dialogScreen, "❯ idle"}}, true, Handled, false, []string{"\x1b[B", "\r"}, 2},
		{"handled late", &fakeScreen{screens: []string{dialogScreen, dialogScreen, dialogScreen, "❯ idle"}}, true, Handled, false, []string{"\x1b[B", "\r"}, 4},
		{"timeout", &fakeScreen{screens: []string{dialogScreen}}, true, Timeout, false, []string{"\x1b[B", "\r"}, 21},
		{"read error", &fakeScreen{screens: []string{dialogScreen}, readErr: boom}, true, "", true, nil, 0},
		{"send error", &fakeScreen{screens: []string{dialogScreen}, sendErr: boom}, true, "", true, []string{"\x1b[B"}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Handle(dialog, tt.screen, fakeClock(), tt.trusted)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("got %q, %v; want %q (err %v)", got, err, tt.want, tt.wantErr)
			}
			if !reflect.DeepEqual(tt.screen.sent, tt.wantSent) || tt.screen.reads != tt.wantReads {
				t.Errorf("sent %q reads %d; want %q reads %d", tt.screen.sent, tt.screen.reads, tt.wantSent, tt.wantReads)
			}
		})
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestTrusted(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	repo, linked, dispatch := filepath.Join(root, "repo"), filepath.Join(root, "linked"), filepath.Join(root, "dispatch")
	for _, d := range []string{repo, dispatch, filepath.Join(repo, "src"), filepath.Join(root, "repo-evil")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	git(t, repo, "init", "-q")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "init", "--no-gpg-sign")
	git(t, repo, "worktree", "add", "-q", "-b", "x", linked)
	if err := os.Symlink(repo, filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	p := Policy{Primary: repo, DispatchDir: dispatch, Extra: []string{filepath.Join(root, "extra")}}
	for dir, want := range map[string]bool{
		repo:                             true,
		linked:                           true,
		dispatch:                         true,
		filepath.Join(root, "alias"):     true,
		filepath.Join(root, "extra"):     true,
		filepath.Join(repo, "src"):       false,
		filepath.Join(dispatch, "sub"):   false,
		filepath.Join(root, "repo-evil"): false,
		"":                               false,
	} {
		if p.Trusted(dir) != want {
			t.Errorf("Trusted(%q) != %v", dir, want)
		}
	}
	if (Policy{Primary: filepath.Join(root, "not-a-repo")}).Trusted(repo) {
		t.Error("git failing must trust nothing but the configured folders")
	}
}

func TestSelectorPath(t *testing.T) {
	tests := []struct {
		selector string
		want     string
		ok       bool
	}{
		{"path:/a/b", "/a/b", true},
		{"path:/a/b/", "/a/b", true},
		{"path:/a/../b", "/b", true},
		{"path:rel/x", "rel/x", true},
		{"path:", ".", true},
		{"id:fake-uuid::/a/b", "/a/b", true},
		{"id:fake-uuid::/a::b", "/a::b", true},
		{"id:fake-uuid::", ".", true},
		{"id:fake-uuid", "", false},
		{"wt-1", "", false},
		{"feature/x-1", "", false},
		{"branch:x", "", false},
		{"folder:abc", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.selector, func(t *testing.T) {
			got, ok := SelectorPath(tt.selector)
			if got != tt.want || ok != tt.ok {
				t.Errorf("SelectorPath(%q) = %q, %v; want %q, %v", tt.selector, got, ok, tt.want, tt.ok)
			}
		})
	}
}

// repoWithWorktree makes a primary checkout, a linked worktree, a lookalike
// folder and a symlink to the primary, all under one real root.
func repoWithWorktree(t *testing.T) (root, repo, linked string) {
	t.Helper()
	root, _ = filepath.EvalSymlinks(t.TempDir())
	repo, linked = filepath.Join(root, "repo"), filepath.Join(root, "linked")
	for _, d := range []string{repo, filepath.Join(root, "repo-evil"), filepath.Join(root, "elsewhere")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	git(t, repo, "init", "-q")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "init", "--no-gpg-sign")
	git(t, repo, "worktree", "add", "-q", "-b", "x", linked)
	if err := os.Symlink(repo, filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	return root, repo, linked
}

func TestPrecheck(t *testing.T) {
	root, repo, linked := repoWithWorktree(t)
	dispatch := filepath.Join(root, "dispatch")
	p := Policy{Primary: repo, DispatchDir: dispatch}
	tests := []struct {
		selector string
		want     string
	}{
		{"", ""},
		{"wt-1", ""},
		{"feature/x-1", ""},
		{"id:fake-uuid", ""},
		{"branch:x", ""},
		{"path:" + repo, ""},
		{"path:" + linked, ""},
		{"path:" + linked + "/", ""},
		{"path:" + filepath.Join(root, "alias"), ""},
		{"id:fake-uuid::" + linked, ""},
		{"path:" + dispatch, ""},
		{"path:rel/x", "--worktree path:rel/x must name an absolute path"},
		{"path:", "--worktree path: must name an absolute path"},
		{"id:fake-uuid::rel", "--worktree id:fake-uuid::rel must name an absolute path"},
		{"path:" + filepath.Join(root, "elsewhere"), "--worktree path:" + filepath.Join(root, "elsewhere") + " names " + filepath.Join(root, "elsewhere") + ", which is not a worktree of this repo"},
		{"path:" + filepath.Join(root, "repo-evil"), "--worktree path:" + filepath.Join(root, "repo-evil") + " names " + filepath.Join(root, "repo-evil") + ", which is not a worktree of this repo"},
		{"path:" + filepath.Join(repo, "src"), "--worktree path:" + filepath.Join(repo, "src") + " names " + filepath.Join(repo, "src") + ", which is not a worktree of this repo"},
		{"id:fake-uuid::" + filepath.Join(root, "elsewhere"), "--worktree id:fake-uuid::" + filepath.Join(root, "elsewhere") + " names " + filepath.Join(root, "elsewhere") + ", which is not a worktree of this repo"},
	}
	for _, tt := range tests {
		t.Run(tt.selector, func(t *testing.T) {
			if got := p.Precheck(tt.selector); got != tt.want {
				t.Errorf("Precheck(%q) = %q; want %q", tt.selector, got, tt.want)
			}
		})
	}
}

func TestPlace(t *testing.T) {
	root, repo, linked := repoWithWorktree(t)
	dispatch := filepath.Join(root, "dispatch")
	elsewhere := filepath.Join(root, "elsewhere")
	p := Policy{Primary: repo, DispatchDir: dispatch}
	general := func(folder string) string {
		return "untrusted folder " + folder + ": a launch runs only in a worktree of this repo; the terminal was closed"
	}
	asked := func(folder, path string) string {
		return "untrusted folder " + folder + ": --worktree asked for " + path + "; the terminal was closed"
	}
	tests := []struct {
		name     string
		reported string
		selector string
		want     string
	}{
		{"primary, no selector", repo, "", ""},
		{"linked, no selector", linked, "", ""},
		{"symlink to the primary", filepath.Join(root, "alias"), "", ""},
		{"dispatcher folder", dispatch, "", ""},
		{"name selector, a worktree", linked, "wt-1", ""},
		{"id without a path, a worktree", repo, "id:fake-uuid", ""},
		{"path selector honoured", linked, "path:" + linked, ""},
		{"path selector through a symlink", repo, "path:" + filepath.Join(root, "alias"), ""},
		{"id selector honoured", linked, "id:fake-uuid::" + linked, ""},
		{"nothing reported", "", "", general("(unknown)")},
		{"nothing reported, path selector", "", "path:" + linked, asked("(unknown)", linked)},
		{"elsewhere", elsewhere, "", general(elsewhere)},
		{"lookalike of the primary", filepath.Join(root, "repo-evil"), "", general(filepath.Join(root, "repo-evil"))},
		{"subfolder of the primary", filepath.Join(repo, "src"), "", general(filepath.Join(repo, "src"))},
		{"subfolder of the dispatcher folder", filepath.Join(dispatch, "sub"), "", general(filepath.Join(dispatch, "sub"))},
		{"name selector, elsewhere", elsewhere, "feature/x-1", general(elsewhere)},
		{"path selector, another worktree", repo, "path:" + linked, asked(repo, linked)},
		{"id selector, another worktree", repo, "id:fake-uuid::" + linked, asked(repo, linked)},
		{"path selector, outside the repo", elsewhere, "path:" + linked, asked(elsewhere, linked)},
		{"path as given, not its real path", elsewhere, "path:" + filepath.Join(root, "alias") + "/", asked(elsewhere, filepath.Join(root, "alias"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.Place(tt.reported, tt.selector); got != tt.want {
				t.Errorf("Place(%q, %q) = %q; want %q", tt.reported, tt.selector, got, tt.want)
			}
		})
	}
	if got := (Policy{Primary: filepath.Join(root, "not-a-repo"), Extra: []string{elsewhere}}).Place(repo, ""); got != general(repo) {
		t.Errorf("git failing must allow only the configured folders, got %q", got)
	}
}
