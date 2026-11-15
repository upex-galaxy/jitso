package hook

import (
	"reflect"
	"strings"
	"testing"
)

func isOwnAt(own string) func(string) bool {
	return func(word string) bool { return word == own }
}

func TestSegments(t *testing.T) {
	tests := []struct {
		name, in string
		want     []string
	}{
		{"empty", "", nil},
		{"one", "ls -la", []string{"ls -la"}},
		{"separators", "a; b && c || d | e & f\ng", []string{"a", "b", "c", "d", "e", "f", "g"}},
		{"substitution and backtick", "echo $(x) `y` (z)", []string{"echo", "x", "y", "z"}},
		{"quotes are removed", `orca "orchestration" 'worker-start'`, []string{"orca orchestration worker-start"}},
		{"whitespace is collapsed", "a \t  b c", []string{"a b c"}},
		{"only separators", ";;&&||", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Segments(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Segments(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestLauncherIs(t *testing.T) {
	l := Launcher{IsOwn: isOwnAt("/opt/jitso")}
	tests := []struct {
		segment string
		want    bool
	}{
		{"jitso launch", true},
		{"jitso launch --build p", true},
		{"bun run jitso launch --build p", true},
		{"/opt/jitso launch --build p", true},
		{"FOO=bar BAZ=a=b jitso launch", true},
		{"FOO=bar /opt/jitso launch", true},
		{"jitso", false},
		{"jitso launcher", false},
		{"jitso close", false},
		{"bun run jitso", false},
		{"bun run other launch", false},
		{"/other/jitso launch", false},
		{"FOO= jitso launch", false},
		{"=a jitso launch", false},
		{"echo jitso launch", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.segment, func(t *testing.T) {
			if got := l.Is(tt.segment); got != tt.want {
				t.Errorf("Is(%q) = %v, want %v", tt.segment, got, tt.want)
			}
		})
	}
}

func TestLauncherBashVerb(t *testing.T) {
	l := Launcher{IsOwn: isOwnAt("/opt/jitso")}
	tests := []struct {
		name, command, want string
	}{
		{"plain ls", "ls", ""},
		{"git and ls", "git status && ls", ""},
		{"echo claude", "echo claude", ""},
		{"launch alone", "/opt/jitso launch --build p --brief b --mode direct", ""},
		{"worker-start", "orca orchestration worker-start --run r1", "orchestration worker-start"},
		{"worker-start with extra spaces", "orca   orchestration    worker-start", "orchestration worker-start"},
		{"worker-start with tab", "orca orchestration\tworker-start", "orchestration worker-start"},
		{"worker-start after launch", "/opt/jitso launch --mode direct; orca orchestration worker-start", "orchestration worker-start"},
		{"worker-start quoted", `bash -c 'orca orchestration worker-start --run r1'`, "orchestration worker-start"},
		{"empty assignment is not a prefix", "FOO= orca orchestration worker-start", "orchestration worker-start"},
		{"dispatch", "orca orchestration dispatch --task t", "orchestration dispatch"},
		{"dispatch at the end", "orca orchestration dispatch", "orchestration dispatch"},
		{"dispatch-like word", "orca orchestration dispatch-all", ""},
		{"dispatch with a word character after", "orca orchestration dispatcher", ""},
		{"dispatch then another dispatch", "orca orchestration dispatcher; orca orchestration dispatch", "orchestration dispatch"},
		{"agent launch", "orca agent launch --agent claude", "agent launch"},
		{"worktree create without agent", "orca worktree create --name x", ""},
		{"worktree create with agent", "orca worktree create --name x --agent claude", "worktree create --agent"},
		{"worktree create with agent=", "orca worktree create --name x --agent=claude", "worktree create --agent"},
		{"worktree create with another flag", "orca worktree create --agent-x y", ""},
		{"terminal create plain", "orca terminal create --worktree w --title t", ""},
		{"terminal create bash", "orca terminal create --worktree w --command bash", ""},
		{"terminal create claude", `orca terminal create --worktree w --command "claude --model x"`, "terminal create --command <harness>"},
		{"terminal create codex path", "orca terminal create --command /usr/local/bin/codex", "terminal create --command <harness>"},
		{"terminal create command=", "orca terminal create --worktree w --command=claude", "terminal create --command <harness>"},
		{"terminal create harness as 3rd word", "orca terminal create --command env X=1 pi", "terminal create --command <harness>"},
		{"terminal create harness as 4th word", "orca terminal create --command env X=1 Y=2 pi", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := l.BashVerb(tt.command); got != tt.want {
				t.Errorf("BashVerb(%q) = %q, want %q", tt.command, got, tt.want)
			}
		})
	}
}

// RE2 is linear, but the cases pin it: a 150000 character input must finish.
func TestBashVerbLongInput(t *testing.T) {
	l := Launcher{IsOwn: isOwnAt("/opt/jitso")}
	for _, command := range []string{
		strings.Repeat("=", 150000) + " orca orchestration worker-start --run r1",
		strings.Repeat("=a", 75000) + " launch",
		strings.Repeat("a=b ", 40000) + "jitso launch",
	} {
		l.BashVerb(command)
	}
	if got := l.BashVerb(strings.Repeat("a=b ", 40000) + "jitso launch"); got != "" {
		t.Errorf("a long assignment prefix before the launcher should pass, got %q", got)
	}
}
