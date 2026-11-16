package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/upex-galaxy/jitso/internal/registry"
)

const launchLine = `{"v":2,"event":"launch","id":"L20261113T142233-a3f9","ts":"2026-11-13T14:22:33.000Z","session_id":"sess-1"}`

func TestEndedBy(t *testing.T) {
	now := time.Date(2026, 11, 13, 14, 30, 0, 0, time.UTC)
	tests := []struct {
		name, text, session string
		wantID              string
		wantDuration        int64
	}{
		{"matching launch", launchLine + "\n", "sess-1", "L20261113T142233-a3f9", 447},
		{"corrupt line is skipped", launchLine + "\nnot json\n", "sess-1", "L20261113T142233-a3f9", 447},
		{"unknown session", launchLine + "\n", "nobody", "", 0},
		{"empty session", launchLine + "\n", "", "", 0},
		{"already ended by anything", launchLine + "\n" + `{"v":2,"event":"end","id":"L20261113T142233-a3f9","source":"cli"}` + "\n", "sess-1", "", 0},
		{"another launch ended", launchLine + "\n" + `{"v":2,"event":"end","id":"L20261113T142233-b4a0"}` + "\n", "sess-1", "L20261113T142233-a3f9", 447},
		{"last launch of the session wins", launchLine + "\n" + `{"v":2,"event":"launch","id":"L20261113T142233-b4a0","ts":"2026-11-13T14:29:00.000Z","session_id":"sess-1"}` + "\n", "sess-1", "L20261113T142233-b4a0", 60},
		{"unreadable ts", `{"v":2,"event":"launch","id":"L20261113T142233-a3f9","ts":"x","session_id":"sess-1"}` + "\n", "sess-1", "L20261113T142233-a3f9", 0},
		{"launch in the future", `{"v":2,"event":"launch","id":"L20261113T142233-a3f9","ts":"2026-11-13T15:00:00.000Z","session_id":"sess-1"}` + "\n", "sess-1", "L20261113T142233-a3f9", 0},
		{"non-object lines", "null\n[]\n42\n" + launchLine + "\n", "sess-1", "L20261113T142233-a3f9", 447},
		{"id that cannot be a key", `{"v":2,"event":"end","id":[1]}` + "\n" + launchLine + "\n", "sess-1", "L20261113T142233-a3f9", 447},
		{"empty registry", "", "sess-1", "", 0},
		{"a v1 launch is not a launch", strings.Replace(launchLine, `"v":2`, `"v":1`, 1) + "\n", "sess-1", "", 0},
		{"a launch with no v is not a launch", strings.Replace(launchLine, `"v":2,`, "", 1) + "\n", "sess-1", "", 0},
		{"a v3 launch is not a launch", strings.Replace(launchLine, `"v":2`, `"v":3`, 1) + "\n", "sess-1", "", 0},
		{"a v1 launch beside a v2 launch of the same session", strings.Replace(launchLine, `"v":2`, `"v":1`, 1) + "\n" + `{"v":2,"event":"launch","id":"L20261113T142233-b4a0","ts":"2026-11-13T14:29:00.000Z","session_id":"sess-1"}` + "\n", "sess-1", "L20261113T142233-b4a0", 60},
		{"a v1 end does not end a v2 launch", launchLine + "\n" + `{"v":1,"event":"end","id":"L20261113T142233-a3f9","source":"jitso_close"}` + "\n", "sess-1", "L20261113T142233-a3f9", 447},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EndedBy([]byte(tt.text), tt.session, now)
			if tt.wantID == "" {
				if got != nil {
					t.Fatalf("EndedBy = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("EndedBy = nil, want an end event")
			}
			if got.ID != tt.wantID || got.DurationS != tt.wantDuration || got.Source != "session_end_hook" || got.OutcomeReported != "none" || got.FooterPath != nil || got.TS != "2026-11-13T14:30:00.000Z" {
				t.Errorf("EndedBy = %+v", got)
			}
		})
	}
}

func TestEndLaunchWritesOneLineAndNoLock(t *testing.T) {
	dir := t.TempDir()
	store := registry.NewFileStore(dir)
	file := store.Path()
	now := time.Date(2026, 11, 13, 14, 30, 0, 0, time.UTC)

	if err := EndLaunch(store, file, "sess-1", now); err != nil {
		t.Fatalf("no registry: %v", err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("a missing registry must stay missing, stat error %v", err)
	}
	if err := os.WriteFile(file, []byte(launchLine+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := EndLaunch(store, file, "sess-1", now); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(file)
	events := registry.Parse(data)
	var ends int
	for _, e := range events {
		if e.Kind() == registry.KindEnd {
			ends++
		}
	}
	if ends != 1 {
		t.Errorf("%d end events, want 1 (registry %q)", ends, data)
	}
	if _, err := os.Stat(filepath.Join(dir, registry.LockName)); !os.IsNotExist(err) {
		t.Errorf("the lock was left behind: %v", err)
	}
}
