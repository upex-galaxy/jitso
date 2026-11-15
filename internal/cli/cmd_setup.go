package cli

import (
	"sort"
	"strings"
)

// `jitso setup <what>` is a group: each subcommand lives in its own
// cmd_setup_<name>.go and registers itself from an init function
// (registerSetupSub), so adding one never edits another's file. The group's
// usage and its "missing what to set up (...)" line are built from what is
// registered, at the moment they are printed.
//
//	setup                  "jitso setup: missing what to set up (<names>)", a blank line, the usage, exit 2
//	setup --help | -h      the usage on stdout, exit 0
//	setup <unknown>        `jitso setup: unknown subcommand "<x>"`, a blank line, the usage, exit 2
//	setup <what> ...       that subcommand
type setupSub struct {
	// Name is the word that selects the subcommand.
	Name string
	// Order places it in the usage (ascending).
	Order int
	// UsageLine is its usage, after "jitso setup <name> ".
	UsageLine string
	// Help is its paragraph in the usage: what it does and its flags.
	Help string
	// Run executes it with the words after its name.
	Run func(ctx *Context, args []string) error
}

var setupSubs []setupSub

// registerSetupSub adds a subcommand to `jitso setup`.
func registerSetupSub(s setupSub) {
	if s.Name == "" || s.Run == nil {
		panic("cli: a setup subcommand needs a name and a Run function")
	}
	for _, have := range setupSubs {
		if have.Name == s.Name {
			panic("cli: setup subcommand " + s.Name + " registered twice")
		}
	}
	setupSubs = append(setupSubs, s)
}

// setupOrdered is the registered subcommands in usage order.
func setupOrdered() []setupSub {
	list := append([]setupSub(nil), setupSubs...)
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Order != list[j].Order {
			return list[i].Order < list[j].Order
		}
		return list[i].Name < list[j].Name
	})
	return list
}

// setupUsage is the usage text of `jitso setup`.
func setupUsage() string {
	var b strings.Builder
	var helps []string
	for i, s := range setupOrdered() {
		if i == 0 {
			b.WriteString("Usage: ")
		} else {
			b.WriteString("       ")
		}
		b.WriteString("jitso setup " + s.Name + " " + s.UsageLine + "\n")
		helps = append(helps, s.Help)
	}
	return b.String() + "\n" + strings.Join(helps, "\n\n")
}

// setupGroup is the group as it stands now.
func setupGroup() Group {
	var names []string
	g := Group{Name: "setup", Usage: setupUsage()}
	for _, s := range setupOrdered() {
		names = append(names, s.Name)
		g.Subs = append(g.Subs, GroupSub{Name: s.Name, Run: s.Run})
	}
	g.Missing = "missing what to set up (" + strings.Join(names, ", ") + ")"
	return g
}

func init() {
	Register(Command{
		Name:    "setup",
		Order:   82,
		Summary: "register the jitso guard hook in this project",
		Run: func(ctx *Context, args []string) error {
			return setupGroup().Run(ctx, args)
		},
	})
}
