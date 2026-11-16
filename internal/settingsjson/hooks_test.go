package settingsjson

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

const canonical = `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "jitso hook guard"
          }
        ]
      },
      {
        "matcher": "mcp__.*orca.*__.*",
        "hooks": [
          {
            "type": "command",
            "command": "jitso hook guard"
          }
        ]
      },
      {
        "matcher": "Write|Edit|MultiEdit|NotebookEdit",
        "hooks": [
          {
            "type": "command",
            "command": "jitso hook guard"
          }
        ]
      }
    ],
    "SessionEnd": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "jitso hook guard"
          }
        ]
      }
    ]
  }
}
`

// grp is a 2-space group text at the depth of an event array item, for
// building expected documents.
func grp(matcher, command string) string {
	m := ""
	if matcher != "" {
		m = "        \"matcher\": \"" + matcher + "\",\n"
	}
	return "      {\n" + m +
		"        \"hooks\": [\n" +
		"          {\n" +
		"            \"type\": \"command\",\n" +
		"            \"command\": \"" + command + "\"\n" +
		"          }\n" +
		"        ]\n" +
		"      }"
}

func register(t *testing.T, doc, command string) string {
	t.Helper()
	var in []byte
	if doc != "<absent>" {
		in = []byte(doc)
	}
	out, err := Register(in, command)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	return string(out)
}

func TestRegisterAbsentIsTheCanonicalFile(t *testing.T) {
	if got := register(t, "<absent>", Command); got != canonical {
		t.Errorf("got:\n%s\nwant:\n%s", got, canonical)
	}
	if got := string(Fragment(Command)); got != canonical {
		t.Errorf("Fragment differs from the canonical file:\n%s", got)
	}
}

func TestRegisterTable(t *testing.T) {
	user := func(matcher, command string) string { return grp(matcher, command) }
	tests := []struct {
		name string
		doc  string
		want string
	}{
		{"an empty object gets the default indent", "{}\n", canonical},
		{
			"user keys and groups stay, JITSO groups follow the user's",
			"{\n  \"$schema\": \"s\",\n  \"hooks\": {\n    \"PreToolUse\": [\n" + user("Bash", "./check.sh") + "\n    ],\n    \"Stop\": [\n" + user("", "./n.sh") + "\n    ]\n  },\n  \"env\": {\n    \"A\": \"1\"\n  }\n}\n",
			"{\n  \"$schema\": \"s\",\n  \"hooks\": {\n    \"PreToolUse\": [\n" + user("Bash", "./check.sh") + ",\n" + grp("Bash", Command) + ",\n" + grp("mcp__.*orca.*__.*", Command) + ",\n" + grp(MatcherWrite, Command) + "\n    ],\n    \"Stop\": [\n" + user("", "./n.sh") + "\n    ],\n    \"SessionEnd\": [\n" + grp("", Command) + "\n    ]\n  },\n  \"env\": {\n    \"A\": \"1\"\n  }\n}\n",
		},
		{
			"hooks is created at the end of the root",
			"{\n  \"model\": \"opus\"\n}\n",
			strings.Replace(canonical, "{\n  \"hooks\"", "{\n  \"model\": \"opus\",\n  \"hooks\"", 1),
		},
		{
			"a legacy absolute command is replaced in place",
			"{\n  \"hooks\": {\n    \"PreToolUse\": [\n" + grp("Bash", "'/old/place/jitso' hook guard") + "\n    ],\n    \"Stop\": [\n" + user("", "./n.sh") + "\n    ],\n    \"SessionEnd\": [\n" + grp("", "/old/jitso hook guard") + "\n    ]\n  }\n}\n",
			"{\n  \"hooks\": {\n    \"PreToolUse\": [\n" + grp("Bash", Command) + ",\n" + grp("mcp__.*orca.*__.*", Command) + ",\n" + grp(MatcherWrite, Command) + "\n    ],\n    \"Stop\": [\n" + user("", "./n.sh") + "\n    ],\n    \"SessionEnd\": [\n" + grp("", Command) + "\n    ]\n  }\n}\n",
		},
		{
			"duplicate JITSO groups collapse and a user handler beside a JITSO one stays",
			"{\n  \"hooks\": {\n    \"PreToolUse\": [\n" +
				"      {\n        \"matcher\": \"Bash\",\n        \"hooks\": [\n          {\n            \"type\": \"command\",\n            \"command\": \"./a.sh\"\n          },\n          {\n            \"type\": \"command\",\n            \"command\": \"jitso hook guard\"\n          }\n        ]\n      },\n" +
				grp("Bash", Command) + ",\n" + grp("Bash", Command) + "\n    ]\n  }\n}\n",
			"{\n  \"hooks\": {\n    \"PreToolUse\": [\n" +
				"      {\n        \"matcher\": \"Bash\",\n        \"hooks\": [\n          {\n            \"type\": \"command\",\n            \"command\": \"./a.sh\"\n          }\n        ]\n      },\n" +
				grp("Bash", Command) + ",\n" + grp("mcp__.*orca.*__.*", Command) + ",\n" + grp(MatcherWrite, Command) + "\n    ],\n    \"SessionEnd\": [\n" + grp("", Command) + "\n    ]\n  }\n}\n",
		},
		{
			"a JITSO handler under another event is removed and the event it empties goes",
			"{\n  \"hooks\": {\n    \"Stop\": [\n" + grp("", Command) + "\n    ]\n  }\n}\n",
			canonical,
		},
		{
			"an empty array the user left stays",
			"{\n  \"hooks\": {\n    \"Stop\": []\n  }\n}\n",
			strings.Replace(canonical, "\"hooks\": {\n", "\"hooks\": {\n    \"Stop\": [],\n", 1),
		},
		{
			"a user group with no hooks key is left alone",
			"{\n  \"hooks\": {\n    \"Stop\": [\n      {\n        \"matcher\": \"x\"\n      }\n    ]\n  }\n}\n",
			strings.Replace(canonical, "\"hooks\": {\n", "\"hooks\": {\n    \"Stop\": [\n      {\n        \"matcher\": \"x\"\n      }\n    ],\n", 1),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := register(t, tt.doc, Command)
			if got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
			// idempotent: the result is the desired state
			if again := register(t, got, Command); again != got {
				t.Errorf("a second Register changed the document:\n%s", again)
			}
		})
	}
}

func TestRegisterAlreadyDesiredReturnsTheSameBytesWhateverTheLayout(t *testing.T) {
	foreign := strings.Replace(canonical, `"hooks": {`, `"x": 1, "hooks": {`, 1)
	out, err := Register([]byte(foreign), Command)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != foreign {
		t.Errorf("the document changed:\n%s", out)
	}
}

func TestRegisterFindsAnAbsoluteCommandItWrote(t *testing.T) {
	cmd := "'/opt/tools/runner' hook guard" // not named jitso: only the exact command identifies it
	once := register(t, "<absent>", cmd)
	if twice := register(t, once, cmd); twice != once {
		t.Errorf("not idempotent for a binary with another name:\n%s", twice)
	}
	if strings.Count(once, cmd) != 4 {
		t.Errorf("want 4 handlers with the command:\n%s", once)
	}
}

func TestRegisterSwitchesTheCommand(t *testing.T) {
	abs := register(t, "<absent>", "'/usr/local/bin/jitso' hook guard")
	if got := register(t, abs, Command); got != canonical {
		t.Errorf("an absolute registration is not replaced by the bare word:\n%s", got)
	}
}

func TestRegisterRefusals(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		kind Kind
		in   string // a fragment of the message
	}{
		{"empty file", "", KindSyntax, "is not valid JSON ("},
		{"open brace", "{", KindSyntax, "is not valid JSON ("},
		{"trailing comma", `{"a": 1,}`, KindSyntax, "is not valid JSON ("},
		{"trailing bytes", "{} x", KindSyntax, "is not valid JSON ("},
		{"two documents", "{}{}", KindSyntax, "is not valid JSON ("},
		{"a comment", "{\n  // x\n}\n", KindSyntax, "is not valid JSON ("},
		{"root array", "[]", KindNotObject, "must hold a JSON object"},
		{"root string", `"x"`, KindNotObject, "must hold a JSON object"},
		{"hooks array", `{"hooks": []}`, KindWrongType, `"hooks" in f must be an object`},
		{"hooks null", `{"hooks": null}`, KindWrongType, `"hooks" in f must be an object`},
		{"event object", `{"hooks": {"PreToolUse": {}}}`, KindWrongType, `"hooks.PreToolUse" in f must be an array`},
		{"group string", `{"hooks": {"Stop": [1]}}`, KindWrongType, `"hooks.Stop.0" in f must be an object`},
		{"group hooks object", `{"hooks": {"Stop": [{}, {"hooks": {}}]}}`, KindWrongType, `"hooks.Stop.1.hooks" in f must be an array`},
		{"duplicate at the root", `{"a":1,"a":2}`, KindDuplicateKey, `f has the key "a" twice at "."`},
		{"duplicate nested", `{"hooks":{"Stop":[{"x":1,"x":2}]}}`, KindDuplicateKey, `f has the key "x" twice at "hooks.Stop.0"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, op := range []string{"register", "unregister"} {
				var err error
				if op == "register" {
					_, err = Register([]byte(tt.doc), Command)
				} else {
					_, err = Unregister([]byte(tt.doc))
				}
				var p *Problem
				if !errors.As(err, &p) || p.Kind != tt.kind {
					t.Fatalf("%s: err = %v, want kind %d", op, err, tt.kind)
				}
				if msg := p.Message("f"); !strings.Contains(msg, tt.in) || !strings.Contains(msg, "nothing written") {
					t.Errorf("%s: message %q lacks %q or `nothing written`", op, msg, tt.in)
				}
			}
		})
	}
}

func TestForeignLayoutsAreRefusedWhenAnEditIsNeeded(t *testing.T) {
	docs := map[string]string{
		"several keys on a line":  "{\n  \"a\": 1, \"b\": 2\n}\n",
		"indent jumps":            "{\n  \"a\": {\n      \"b\": 1\n  }\n}\n",
		"crlf":                    "{\r\n  \"a\": 1\r\n}\r\n",
		"unicode escape":          "{\n  \"a\": \"\\u00e9\"\n}\n",
		"two final newlines":      "{\n  \"a\": 1\n}\n\n",
		"space before colon":      "{\n  \"a\" : 1\n}\n",
		"mixed tab and space":     "{\n  \"a\": {\n\t\"b\": 1\n  }\n}\n",
		"a comma on its own line": "{\n  \"a\": [\n    1\n  ,2]\n}\n",
		"escaped slash":           "{\n  \"a\": \"x\\/y\"\n}\n",
	}
	for name, doc := range docs {
		t.Run(name, func(t *testing.T) {
			_, err := Register([]byte(doc), Command)
			var p *Problem
			if !errors.As(err, &p) || p.Kind != KindLayout {
				t.Fatalf("Register err = %v, want a layout problem", err)
			}
			if !strings.Contains(p.Message("f"), "is not laid out the way jitso writes JSON") {
				t.Errorf("message: %s", p.Message("f"))
			}
		})
	}
	// Unregister of a foreign file with no JITSO handler needs no edit.
	if out, err := Unregister([]byte(docs["several keys on a line"])); err != nil || string(out) != docs["several keys on a line"] {
		t.Errorf("Unregister of a file with nothing to remove: %q, %v", out, err)
	}
	// ... but one with a JITSO handler does.
	foreign := strings.Replace(canonical, "\"hooks\": {\n", "\"a\": 1, \"b\": 2,\n  \"hooks\": {\n", 1)
	if _, err := Unregister([]byte(foreign)); err == nil {
		t.Error("Unregister edited a file in a foreign layout")
	}
}

// Every layout the edit keeps: Register then Unregister gives the same bytes.
func TestRoundTripCorpus(t *testing.T) {
	user := `"hooks":{"Stop":[{"hooks":[{"type":"command","command":"./n.sh"}]}]}`
	compact := `{"model":"opus",` + user + "}\n"
	tests := []struct{ name, doc string }{
		{"two spaces", "{\n  \"hooks\": {\n    \"Stop\": [\n" + grp("", "./n.sh") + "\n    ]\n  },\n  \"env\": {\n    \"K\": \"v\"\n  }\n}\n"},
		{"four spaces", "{\n    \"model\": \"opus\",\n    \"hooks\": {\n        \"Stop\": [\n            {\n                \"hooks\": [\n                    {\n                        \"type\": \"command\",\n                        \"command\": \"./n.sh\"\n                    }\n                ]\n            }\n        ]\n    }\n}\n"},
		{"tab", "{\n\t\"model\": \"opus\",\n\t\"hooks\": {\n\t\t\"Stop\": [\n\t\t\t{\n\t\t\t\t\"hooks\": [\n\t\t\t\t\t{\n\t\t\t\t\t\t\"type\": \"command\",\n\t\t\t\t\t\t\"command\": \"./n.sh\"\n\t\t\t\t\t}\n\t\t\t\t]\n\t\t\t}\n\t\t]\n\t}\n}\n"},
		{"compact", compact},
		{"compact without a newline", strings.TrimSuffix(compact, "\n")},
		{"numbers kept as written", "{\n  \"ratio\": 1.50,\n  \"big\": 12345678901234567890,\n  \"exp\": 1E+5,\n  \"neg\": -0.0,\n  \"hooks\": {\n    \"Stop\": [\n" + grp("", "./n.sh") + "\n    ]\n  }\n}\n"},
		{"unicode", "{\n  \"name\": \"café → ok 日本語 😀\",\n  \"hooks\": {\n    \"Stop\": [\n" + grp("", "./é.sh") + "\n    ]\n  }\n}\n"},
		{"escaped characters", "{\n  \"s\": \"a\\\"b\\\\c\\nd\\te <x> & y\\u0001\",\n  \"hooks\": {\n    \"Stop\": [\n" + grp("", "./n.sh") + "\n    ]\n  }\n}\n"},
		{"no final newline", strings.TrimSuffix("{\n  \"hooks\": {\n    \"Stop\": [\n"+grp("", "./n.sh")+"\n    ]\n  }\n}\n", "\n")},
		{"empty containers elsewhere", "{\n  \"permissions\": {},\n  \"list\": [],\n  \"hooks\": {\n    \"Stop\": [\n" + grp("", "./n.sh") + "\n    ]\n  }\n}\n"},
		{"null and booleans", "{\n  \"a\": null,\n  \"b\": true,\n  \"c\": false,\n  \"hooks\": {\n    \"Stop\": [\n" + grp("", "./n.sh") + "\n    ]\n  }\n}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, p := parse([]byte(tt.doc))
			if p != nil {
				t.Fatalf("parse: %v", p)
			}
			if p := d.checkLayout(); p != nil {
				t.Fatalf("the corpus entry does not re-encode to its own bytes")
			}
			with := register(t, tt.doc, Command)
			if with == tt.doc {
				t.Fatal("Register changed nothing")
			}
			if !State([]byte(with)).Registered() {
				t.Errorf("the guard is not registered:\n%s", with)
			}
			back, err := Unregister([]byte(with))
			if err != nil {
				t.Fatalf("Unregister: %v", err)
			}
			if string(back) != tt.doc {
				t.Errorf("round trip changed the bytes:\n got: %q\nwant: %q", back, tt.doc)
			}
		})
	}
}

func TestUnregisterTable(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want string // "<nil>" means the file is deleted
	}{
		{"only jitso: the file goes", canonical, "<nil>"},
		{
			"other root keys keep the file",
			strings.Replace(canonical, "{\n  \"hooks\"", "{\n  \"model\": \"opus\",\n  \"hooks\"", 1),
			"{\n  \"model\": \"opus\"\n}\n",
		},
		{
			"a user event stays, hooks stays",
			strings.Replace(canonical, "\"hooks\": {\n", "\"hooks\": {\n    \"Stop\": [\n"+grp("", "./n.sh")+"\n    ],\n", 1),
			"{\n  \"hooks\": {\n    \"Stop\": [\n" + grp("", "./n.sh") + "\n    ]\n  }\n}\n",
		},
		{
			"a legacy command is removed too",
			"{\n  \"hooks\": {\n    \"SessionEnd\": [\n" + grp("", "'/x/jitso' hook guard") + "\n    ]\n  }\n}\n",
			"<nil>",
		},
		{
			"a user handler beside a JITSO one stays",
			"{\n  \"hooks\": {\n    \"Stop\": [\n      {\n        \"hooks\": [\n          {\n            \"type\": \"command\",\n            \"command\": \"./a.sh\"\n          },\n          {\n            \"type\": \"command\",\n            \"command\": \"jitso hook guard\"\n          }\n        ]\n      }\n    ]\n  }\n}\n",
			"{\n  \"hooks\": {\n    \"Stop\": [\n      {\n        \"hooks\": [\n          {\n            \"type\": \"command\",\n            \"command\": \"./a.sh\"\n          }\n        ]\n      }\n    ]\n  }\n}\n",
		},
		{"no jitso handler: unchanged", "{\n  \"hooks\": {\n    \"Stop\": []\n  }\n}\n", "{\n  \"hooks\": {\n    \"Stop\": []\n  }\n}\n"},
		{"no hooks: unchanged", "{}\n", "{}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := Unregister([]byte(tt.doc))
			if err != nil {
				t.Fatal(err)
			}
			got := string(out)
			if out == nil {
				got = "<nil>"
			}
			if got != tt.want {
				t.Errorf("got %q\nwant %q", got, tt.want)
			}
		})
	}
	if out, err := Unregister(nil); out != nil || err != nil {
		t.Errorf("an absent file: %q, %v", out, err)
	}
}

func TestIsJitsoCommand(t *testing.T) {
	yes := []string{
		"jitso hook guard",
		"'/usr/local/bin/jitso' hook guard",
		`'/opt/my path/jitso' hook guard`,
		`'/it'\''s/jitso' hook guard`,
		`"/usr/local/bin/jitso" hook guard`,
		"/usr/local/bin/jitso hook guard",
		"'/x/jitso.exe' hook guard",
	}
	no := []string{
		"jitso hook dispatch-guard",
		"jitso hook guard extra",
		"bun run jitso hook guard",
		"./jitso hook guard",
		"jitsox hook guard",
		"/x/other hook guard",
		"echo jitso hook guard",
		" jitso hook guard",
		"",
	}
	for _, c := range yes {
		if !IsJitsoCommand(c) {
			t.Errorf("%q is a JITSO command", c)
		}
	}
	for _, c := range no {
		if IsJitsoCommand(c) {
			t.Errorf("%q is not a JITSO command", c)
		}
	}
}

func TestState(t *testing.T) {
	tests := []struct {
		name string
		doc  []byte
		want HookState
	}{
		{"absent", nil, HookState{}},
		{"registered", []byte(canonical), HookState{Bash: true, MCP: true, Write: true, SessionEnd: true, Commands: []string{Command}}},
		{
			"legacy command on the Bash slot only",
			[]byte("{\n  \"disableAllHooks\": true,\n  \"hooks\": {\n    \"PreToolUse\": [\n" + grp("Bash", "'/x/jitso' hook guard") + "\n    ]\n  }\n}\n"),
			HookState{Bash: true, DisableAllHooks: true, Commands: []string{"'/x/jitso' hook guard"}},
		},
		{
			"a mixed group is a handler but not a slot",
			[]byte("{\n  \"hooks\": {\n    \"SessionEnd\": [\n      {\n        \"hooks\": [\n          {\n            \"type\": \"command\",\n            \"command\": \"./a.sh\"\n          },\n          {\n            \"type\": \"command\",\n            \"command\": \"jitso hook guard\"\n          }\n        ]\n      }\n    ]\n  }\n}\n"),
			HookState{Commands: []string{Command}},
		},
		{"disableAllHooks false", []byte("{\n  \"disableAllHooks\": false\n}\n"), HookState{}},
		{"a wrong matcher is no slot", []byte("{\n  \"hooks\": {\n    \"PreToolUse\": [\n" + grp("Write", Command) + "\n    ]\n  }\n}\n"), HookState{Commands: []string{Command}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := State(tt.doc)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
	s := State([]byte(`{"hooks": []}`))
	var p *Problem
	if !errors.As(s.Err, &p) || s.Registered() {
		t.Errorf("a broken file: %+v", s)
	}
	if !State(nil).None() || State([]byte(canonical)).None() {
		t.Error("None is wrong")
	}
}
