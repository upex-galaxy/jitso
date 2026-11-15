// Package shell is the only place jitso turns data into shell text. Every word
// goes through Quote, so an argument is always one literal word to sh whatever
// it holds. Anything a model controls besides the brief must first pass
// CheckLaunchField: an allowlist of plain characters that never needs escaping.
//
// It holds Quote, the field allowlists, the run.sh text (RunScript), the
// --command word (CommandFor) and the terminal title (TerminalTitle).
package shell

import (
	"errors"
	"regexp"
	"strings"
)

// ErrNUL is returned for a value with a NUL byte, which no argv can carry.
var ErrNUL = errors.New("a NUL byte cannot be part of a command")

// Quote wraps value as one POSIX shell word: the value inside single quotes,
// with each single quote in it closed, escaped with a backslash and reopened.
// It fails on a NUL byte.
func Quote(value string) (string, error) {
	if strings.ContainsRune(value, 0) {
		return "", ErrNUL
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'", nil
}

var plainWord = regexp.MustCompile(`^[\w@%+=:,./-]+$`)

// Word is value as it must be written in a command line a human or a model
// types: unchanged when it holds plain characters only, else one single-quoted
// word (Quote). The dispatch guard reads exactly this form.
func Word(value string) (string, error) {
	if plainWord.MatchString(value) {
		return value, nil
	}
	return Quote(value)
}
