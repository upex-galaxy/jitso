package cli

import (
	"slices"
	"strings"

	"github.com/upex-galaxy/jitso/internal/fail"
)

// FlagSet is a set of the adoption flags, for a command to declare which ones
// it takes.
type FlagSet uint

// The adoption flags.
const (
	FlagScope FlagSet = 1 << iota
	FlagRemove
	FlagForce
	FlagAbsolute
	FlagDryRun
	FlagYes
)

// Flags are the values read by ParseFlags.
type Flags struct {
	// Scope is "user" or "project"; "" when --scope was not given.
	Scope    string
	Remove   bool
	Force    bool
	Absolute bool
	DryRun   bool
	Yes      bool
}

type flagName struct {
	name string
	bit  FlagSet
}

var flagNames = []flagName{
	{"--scope", FlagScope},
	{"--remove", FlagRemove},
	{"--force", FlagForce},
	{"--absolute", FlagAbsolute},
	{"--dry-run", FlagDryRun},
	{"--yes", FlagYes},
}

// ParseFlags reads the words after a subcommand, by hand, with the messages
// `jitso launch` uses. accept is the set this command takes.
//
//   - A word that starts with "-" and is not an accepted flag is
//     `JITSO REJECTED: unknown argument "<word>"` (so is a flag the command
//     does not declare, and --scope=user: the value is the next word).
//   - --scope takes the next word; a missing one, or one that starts with
//     "--", is `JITSO REJECTED: --scope needs a value`; a value other than
//     user or project is `JITSO REJECTED: --scope must be user or project`.
//   - A flag given twice is the same as once; the last --scope wins.
//   - A word that does not start with "-" is positional: it is returned in
//     rest, in order, for the command to accept or reject.
//
// A bad flag or value is the error, the first one reading left to right;
// positional words are never an error here (see RejectExtra), so a stray word
// before a bad flag does not hide it.
func ParseFlags(args []string, accept FlagSet) (flags Flags, rest []string, err error) {
	for i := 0; i < len(args); i++ {
		word := args[i]
		if !strings.HasPrefix(word, "-") {
			rest = append(rest, word)
			continue
		}
		at := slices.IndexFunc(flagNames, func(f flagName) bool { return f.name == word })
		if at < 0 || accept&flagNames[at].bit == 0 {
			return Flags{}, nil, fail.Rejected(`unknown argument "` + word + `"`)
		}
		switch flagNames[at].bit {
		case FlagScope:
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return Flags{}, nil, fail.Rejected("--scope needs a value")
			}
			i++
			if args[i] != "user" && args[i] != "project" {
				return Flags{}, nil, fail.Rejected("--scope must be user or project")
			}
			flags.Scope = args[i]
		case FlagRemove:
			flags.Remove = true
		case FlagForce:
			flags.Force = true
		case FlagAbsolute:
			flags.Absolute = true
		case FlagDryRun:
			flags.DryRun = true
		case FlagYes:
			flags.Yes = true
		}
	}
	return flags, rest, nil
}

// RejectExtra is the answer to a positional word a command does not take:
// `JITSO REJECTED: unknown argument "<first>"`, or nil when there is none.
func RejectExtra(rest []string) error {
	if len(rest) == 0 {
		return nil
	}
	return fail.Rejected(`unknown argument "` + rest[0] + `"`)
}
