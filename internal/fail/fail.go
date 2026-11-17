// Package fail holds the error kinds every jitso package returns and the one
// mapping from an error to an exit code and a stderr text.
//
// It is a leaf package (it imports nothing internal) so that any package can
// return a Rejected or an Env error without importing the command layer.
//
//	Rejected  exit 2  "JITSO REJECTED: <reason>"
//	Usage     exit 2  the text as given
//	Env       exit 3  "jitso: <message>"
//	Status    the given code, nothing printed
//
// Any other error maps to exit 3 with "jitso: <message>", the oracle's
// handling of an uncaught error in a command.
package fail

import (
	"errors"
	"fmt"
)

// Exit codes of the CLI.
const (
	CodeOK       = 0
	CodeRejected = 2
	CodeEnv      = 3
)

// RejectedPrefix starts every validation rejection the CLI prints.
const RejectedPrefix = "JITSO REJECTED: "

// EnvPrefix starts every environment error the CLI prints.
const EnvPrefix = "jitso: "

// Kind tells how an Error is printed and which exit code it carries.
type Kind int

// The error kinds.
const (
	KindRejected Kind = iota + 1
	KindUsage
	KindEnv
	KindStatus
)

// Error is an error that carries its exit code and its stderr text.
type Error struct {
	Kind Kind
	// Msg is the message without its prefix.
	Msg  string
	code int
}

// Error returns the text the CLI prints on stderr.
func (e *Error) Error() string {
	switch e.Kind {
	case KindRejected:
		return RejectedPrefix + e.Msg
	case KindEnv:
		return EnvPrefix + e.Msg
	case KindStatus:
		return fmt.Sprintf("exit status %d", e.code)
	default:
		return e.Msg
	}
}

// RejectedText formats a rejection line: "JITSO REJECTED: <reason>".
func RejectedText(reason string) string { return RejectedPrefix + reason }

// Rejected is a validation rejection: exit 2, "JITSO REJECTED: <reason>".
func Rejected(reason string) *Error {
	return &Error{Kind: KindRejected, Msg: reason, code: CodeRejected}
}

// Rejectedf is Rejected with a format. Do not use %q for values: the oracle
// quotes with plain double quotes and escapes nothing.
func Rejectedf(format string, args ...any) *Error {
	return Rejected(fmt.Sprintf(format, args...))
}

// Usage is a usage error: exit 2, the text printed as is.
func Usage(text string) *Error {
	return &Error{Kind: KindUsage, Msg: text, code: CodeRejected}
}

// Env is an environment or Orca error: exit 3, "jitso: <message>".
func Env(msg string) *Error {
	return &Error{Kind: KindEnv, Msg: msg, code: CodeEnv}
}

// Envf is Env with a format.
func Envf(format string, args ...any) *Error {
	return Env(fmt.Sprintf(format, args...))
}

// Status ends the command with the given exit code and prints nothing: the
// command has already printed its own report.
func Status(code int) *Error {
	return &Error{Kind: KindStatus, code: code}
}

// Code is the exit code for err: 0 for nil, the code of an *Error found with
// errors.As, otherwise 3.
func Code(err error) int {
	if err == nil {
		return CodeOK
	}
	var e *Error
	if errors.As(err, &e) {
		return e.code
	}
	return CodeEnv
}

// Text is what the CLI prints on stderr for err, without a trailing newline.
// It is empty for nil and for a Status error.
func Text(err error) string {
	if err == nil {
		return ""
	}
	var e *Error
	if errors.As(err, &e) {
		if e.Kind == KindStatus {
			return ""
		}
		return e.Error()
	}
	return EnvPrefix + err.Error()
}
