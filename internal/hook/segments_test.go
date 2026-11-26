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

var bashVerbTests = []struct {
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

func TestLauncherBashVerb(t *testing.T) {
	l := Launcher{IsOwn: isOwnAt("/opt/jitso")}
	tests := bashVerbTests
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

var bashDenialTests = []struct {
	name, command string
	want          Denial
}{
	{"build save", "jitso build save x", Denial{}},
	{"own build save", "/opt/jitso build save z", Denial{}},
	{"setup", "jitso setup hook --yes", Denial{}},
	{"install", "bun run jitso install skill --scope user --yes", Denial{}},
	{"init inside sh -c", "sh -c 'jitso init --yes'", Denial{}},
	{"own launch is allowed", "/opt/jitso launch --build b", Denial{}},
	{"a launch after an owner command", "jitso setup hook; orca orchestration worker-start", Denial{Verb: "orchestration worker-start"}},
	{"a launch segment does not shield the next", "jitso launch --build b && claude -p hi", Denial{Verb: "claude"}},
	{"nothing", "ls", Denial{}},
}

func TestBashDenial(t *testing.T) {
	l := Launcher{IsOwn: isOwnAt("/opt/jitso")}
	for _, tt := range bashDenialTests {
		t.Run(tt.name, func(t *testing.T) {
			if got := l.BashDenial(tt.command); got != tt.want {
				t.Errorf("BashDenial(%q) = %+v, want %+v", tt.command, got, tt.want)
			}
		})
	}
}

// movedVerdicts are the rows of the tables above whose verdict the lexer
// changes, with the new verdict: text in quotes or a heredoc body that no shell
// reads. Every other row keeps the verdict of the old splitter.
var movedVerdicts = map[string]Denial{
	"git commit -m 'fix (claude) bug'":  {},
	"git commit -m 'see `claude` docs'": {},
	"cat <<'EOF'\nclaude rules\nEOF":    {},
}

// Every row of the tables goes through the old splitter and through the guard's
// judgment; only the rows of movedVerdicts may differ.
func TestMovedVerdicts(t *testing.T) {
	type row struct {
		l       Launcher
		command string
	}
	var rows []row
	add := func(l Launcher, c string) { rows = append(rows, row{l, c}) }
	opt, none := Launcher{IsOwn: isOwnAt("/opt/jitso")}, Launcher{}
	for _, tt := range bashVerbTests {
		add(opt, tt.command)
	}
	for _, tt := range bashDenialTests {
		add(opt, tt.command)
	}
	for _, tt := range harnessMatrixTests {
		add(none, tt.command)
	}
	for c := range harnessOrderTests {
		add(none, c)
	}
	seen := map[string]bool{}
	for _, r := range rows {
		old, got := r.l.segmentDenial(r.command), r.l.BashDenial(r.command)
		want, moved := movedVerdicts[r.command]
		if !moved {
			want = old
		}
		if got != want {
			t.Errorf("BashDenial(%q) = %+v, want %+v (old splitter %+v)", r.command, got, want, old)
		}
		if moved {
			seen[r.command] = true
			if old == got {
				t.Errorf("%q is listed as moved but keeps %+v", r.command, old)
			}
		}
	}
	for c := range movedVerdicts {
		if !seen[c] {
			t.Errorf("%q is listed as moved but is no row", c)
		}
	}
}
