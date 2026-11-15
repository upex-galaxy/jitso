package version

import (
	"regexp"
	"runtime/debug"
	"testing"
)

func settings(kv ...string) []debug.BuildSetting {
	var out []debug.BuildSetting
	for i := 0; i+1 < len(kv); i += 2 {
		out = append(out, debug.BuildSetting{Key: kv[i], Value: kv[i+1]})
	}
	return out
}

func TestLine(t *testing.T) {
	const rev = "0123456789abcdef0123456789abcdef01234567"
	tests := []struct {
		name    string
		stamped string
		info    *debug.BuildInfo
		ok      bool
		want    string
	}{
		{
			name:    "a stamped version wins over the module version",
			stamped: "v0.1.0",
			info:    &debug.BuildInfo{Main: debug.Module{Version: "v9.9.9"}, Settings: settings("vcs.revision", rev)},
			ok:      true,
			want:    "jitso v0.1.0 (commit 0123456789ab, go1.99, linux/arm64)",
		},
		{
			name: "devel module version and no stamp is dev",
			info: &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: settings("vcs.revision", rev)},
			ok:   true,
			want: "jitso dev (commit 0123456789ab, go1.99, linux/arm64)",
		},
		{
			name: "an empty module version is dev",
			info: &debug.BuildInfo{},
			ok:   true,
			want: "jitso dev (commit unknown, go1.99, linux/arm64)",
		},
		{
			name: "a tagged module version is used when nothing is stamped",
			info: &debug.BuildInfo{Main: debug.Module{Version: "v0.2.0"}},
			ok:   true,
			want: "jitso v0.2.0 (commit unknown, go1.99, linux/arm64)",
		},
		{
			name:    "a modified tree is dirty",
			stamped: "v0.1.0",
			info:    &debug.BuildInfo{Settings: settings("vcs.revision", rev, "vcs.modified", "true")},
			ok:      true,
			want:    "jitso v0.1.0 (commit 0123456789ab-dirty, go1.99, linux/arm64)",
		},
		{
			name:    "an unmodified tree is not dirty",
			stamped: "v0.1.0",
			info:    &debug.BuildInfo{Settings: settings("vcs.revision", rev, "vcs.modified", "false")},
			ok:      true,
			want:    "jitso v0.1.0 (commit 0123456789ab, go1.99, linux/arm64)",
		},
		{
			name: "no build info at all",
			ok:   false,
			want: "jitso dev (commit unknown, go1.99, linux/arm64)",
		},
		{
			name:    "no build info but a stamp",
			stamped: "v0.1.0",
			ok:      false,
			want:    "jitso v0.1.0 (commit unknown, go1.99, linux/arm64)",
		},
		{
			name:    "a short revision is kept whole",
			stamped: "v0.1.0",
			info:    &debug.BuildInfo{Settings: settings("vcs.revision", "abc123")},
			ok:      true,
			want:    "jitso v0.1.0 (commit abc123, go1.99, linux/arm64)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := line(tt.stamped, tt.info, tt.ok, "go1.99", "linux", "arm64"); got != tt.want {
				t.Errorf("line = %q, want %q", got, tt.want)
			}
		})
	}
}

// The real Line has the shape the contract pins.
func TestLineShape(t *testing.T) {
	re := regexp.MustCompile(`^jitso [^ ]+ \(commit (?:[0-9a-f]{12}(?:-dirty)?|unknown), go[^,]+, [a-z0-9]+/[a-z0-9]+\)$`)
	if got := Line(); !re.MatchString(got) {
		t.Errorf("Line() = %q does not match %s", got, re)
	}
}

func TestCurrentIsTheVersionOfLine(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })
	Version = "v7.7.7"
	if got := Current(); got != "v7.7.7" {
		t.Errorf("Current() = %q, want the stamped version", got)
	}
	Version = ""
	if got := Current(); got == "" {
		t.Error("Current() is empty, want at least dev")
	}
}
