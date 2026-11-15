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

func TestBuildSave(t *testing.T) {
	l := Launcher{IsOwn: isOwnAt("/opt/tools/jitso-go")}
	// The command goes through Segments first, as the guard does.
	tests := []struct {
		name, command string
		wantID        string
		want          bool
	}{
		{"bare word", "jitso build save x", "x", true},
		{"bun run", "bun run jitso build save x", "x", true},
		{"a path ending in jitso", "/usr/local/bin/jitso build save x", "x", true},
		{"a relative path", "./jitso build save x", "x", true},
		{"after assignments", "FOO=1 BAR=2 jitso build save x", "x", true},
		{"env prefix", "env FOO=1 jitso build save x", "x", true},
		{"after cd", "cd a && jitso build save x", "x", true},
		{"after a semicolon", "ls; jitso build save x", "x", true},
		{"after a newline", "echo ok\njitso build save x", "x", true},
		{"in a subshell", "(jitso build save x)", "x", true},
		{"in a command substitution", "echo $(jitso build save x)", "x", true},
		{"inside sh -c single quotes", "sh -c 'jitso build save x'", "x", true},
		{"inside bash -c double quotes", `bash -c "jitso build save x"`, "x", true},
		{"padded", "jitso   build    save x", "x", true},
		{"tabs", "jitso\tbuild\tsave\tx", "x", true},
		{"with a token", "jitso build save my-build --confirm abc123abc123", "my-build", true},
		{"token first", "jitso build save --confirm abc123abc123 my-build", "my-build", true},
		{"no id", "jitso build save", "<id>", true},
		{"only flags", "jitso build save --confirm abc", "<id>", true},
		{"an id that is not an id is not echoed", "jitso build save 'Evil Name'", "<id>", true},
		{"a path is not echoed", "jitso build save ../x", "<id>", true},
		{"own path, another name", "/opt/tools/jitso-go build save x", "x", true},
		{"own path after assignments", "A=1 /opt/tools/jitso-go build save y", "y", true},
		{"own path inside sh -c", "sh -c '/opt/tools/jitso-go build save y'", "y", true},
		{"a commit message that holds the words is denied", `git commit -m "jitso build save"`, "<id>", true},
		{"echo of the words is denied", "echo jitso build save x", "x", true},
		{"build draft", "jitso build draft x.yaml", "", false},
		{"build list", "jitso build list", "", false},
		{"build validate", "jitso build validate x", "", false},
		{"build discard", "jitso build discard --all", "", false},
		{"build new", "jitso build new x", "", false},
		{"launch", "jitso launch --build x", "", false},
		{"another word that ends with save", "jitso build saved x", "", false},
		{"a longer word ends in jitso", "my-jitso build save x", "", false},
		{"a file named save", "cat jitso/builds/save.yaml", "", false},
		{"the words in another order", "jitso save build x", "", false},
		{"build with a flag between", "jitso build --x save", "", false},
		{"another program with the own name inside a word", "/opt/tools/jitso-go-x build save x", "", false},
		{"only build save", "echo build save", "", false},
		{"empty", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var id string
			var got bool
			for _, s := range Segments(tt.command) {
				if i, ok := l.BuildSave(s); ok {
					id, got = i, true
					break
				}
			}
			if got != tt.want || id != tt.wantID {
				t.Errorf("BuildSave(%q) = %q, %v; want %q, %v", tt.command, id, got, tt.wantID, tt.want)
			}
		})
	}
}

func TestBashDenial(t *testing.T) {
	l := Launcher{IsOwn: isOwnAt("/opt/jitso")}
	tests := []struct {
		name, command string
		want          Denial
	}{
		{"save", "jitso build save x", Denial{Verb: VerbBuildSave, SaveID: "x"}},
		{"save after a launch segment", "jitso launch --build b && jitso build save y", Denial{Verb: VerbBuildSave, SaveID: "y"}},
		{"launch before save in another order", "orca orchestration worker-start; jitso build save y", Denial{Verb: "orchestration worker-start"}},
		{"own launch is allowed", "/opt/jitso launch --build b", Denial{}},
		{"own save is denied", "/opt/jitso build save z", Denial{Verb: VerbBuildSave, SaveID: "z"}},
		{"nothing", "ls", Denial{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := l.BashDenial(tt.command); got != tt.want {
				t.Errorf("BashDenial(%q) = %+v, want %+v", tt.command, got, tt.want)
			}
		})
	}
}
