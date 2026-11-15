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
