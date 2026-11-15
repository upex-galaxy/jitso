package cli

// install is a group: `jitso install skill`. It copies things jitso ships
// into the places agent harnesses read them from. Every writing subcommand
// goes through internal/adopt: a preview first, then --dry-run, --yes or a
// question on a terminal, a backup of every existing file it changes, and
// --remove as the exact undo.
//
//	install                  "jitso install: missing what to install (skill)", the usage, exit 2
//	install --help | -h      the usage on stdout, exit 0
//	install <unknown>        `jitso install: unknown subcommand "<x>"`, the usage, exit 2
//	install skill ...        cmd_install_skill.go
var installGroup = Group{
	Name:    "install",
	Usage:   installSkillUsage,
	Missing: "missing what to install (skill)",
	Subs: []GroupSub{
		{Name: "skill", Run: runInstallSkill},
	},
}

func init() {
	Register(installGroup.Command(80, "install the jitso usage skill for agent harnesses"))
}
