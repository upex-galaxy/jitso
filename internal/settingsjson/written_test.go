package settingsjson

import "testing"

func TestWrittenLine(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{`"hooks": {`, true},
		{`"hooks": [`, true},
		{`"PreToolUse": [`, true},
		{`"SessionEnd": [`, true},
		{`"matcher": "Bash"`, true},
		{`"matcher": "mcp__.*orca.*__.*"`, true},
		{`"matcher": "Write|Edit|MultiEdit|NotebookEdit"`, false},
		{`"matcher": "Write|Edit"`, false},
		{`"type": "command"`, true},
		{`"command": "jitso hook guard"`, true},
		{`"command": "'/opt/jitso' hook guard"`, true},
		{`"command": "/opt/x/mybin hook guard"`, false}, // true only for WrittenLine of that command
		{`"command": "./notify.sh jitso hook guard SECRET"`, false},
		{`"command": "./notify.sh SECRET"`, false},
		{`"command": "jitso hook guard; echo SECRET"`, false},
		{`"matcher": "Write"`, false},
		{`"apiKeyHelper": "SECRET"`, false},
		{`"Stop": [`, false},
		{``, false},
	}
	for _, tt := range tests {
		if got := WrittenLine("")(tt.line); got != tt.want {
			t.Errorf("WrittenLine(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
	written := WrittenLine("/opt/x/mybin hook guard")
	if !written(`"command": "/opt/x/mybin hook guard"`) {
		t.Error("the command being written must print")
	}
}
