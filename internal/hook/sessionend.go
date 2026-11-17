package hook

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"time"

	"github.com/upex-galaxy/jitso/internal/jsonx"
	"github.com/upex-galaxy/jitso/internal/registry"
)

// EndedBy is the end event a SessionEnd hook owes for sessionID, nil when it
// owes none: the registry text names no launch of that session, or that launch
// already has an end event. Lines are read raw, as the earlier hook did: a
// line that is not JSON is skipped, and a line a strict reader would call
// invalid (an `end` with a source this version never writes) still counts as
// the launch's end. A line whose `v` is not the current schema version is
// skipped like every reader skips it: a v1 launch is not a launch, and an end
// event for it would be an orphan no reader sees.
func EndedBy(registryText []byte, sessionID string, now time.Time) *registry.EndEvent {
	if sessionID == "" {
		return nil
	}
	var launch map[string]any
	ended := map[string]bool{}
	for _, line := range bytes.Split(registryText, []byte("\n")) {
		var event map[string]any
		if json.Unmarshal(line, &event) != nil || event["v"] != float64(registry.Version) {
			continue
		}
		switch event["event"] {
		case "launch":
			if event["session_id"] == sessionID {
				launch = event
			}
		case "end":
			if id, ok := event["id"].(string); ok {
				ended[id] = true
			}
		}
	}
	id, _ := launch["id"].(string)
	if launch == nil || ended[id] {
		return nil
	}
	return &registry.EndEvent{
		ID:              id,
		TS:              jsonx.Time(now),
		Source:          "session_end_hook",
		OutcomeReported: "none",
		DurationS:       durationSeconds(launch["ts"], now),
	}
}

// durationSeconds is max(0, round((now - ts) / 1 s)); an unreadable ts is 0.
func durationSeconds(ts any, now time.Time) int64 {
	s, _ := ts.(string)
	started, ok := registry.ParseTime(s)
	if !ok {
		return 0
	}
	return int64(math.Max(0, math.Round(now.Sub(started).Seconds())))
}

// EndLaunch writes the end event of sessionID's launch through store, when
// registryFile exists and holds one to write. A missing registry is no work,
// and store failures are the caller's to swallow: a registry problem must not
// break the closing of a session.
func EndLaunch(store registry.Store, registryFile, sessionID string, now time.Time) error {
	data, err := os.ReadFile(registryFile)
	if err != nil {
		return nil
	}
	end := EndedBy(data, sessionID, now)
	if end == nil {
		return nil
	}
	return store.Append(end)
}
