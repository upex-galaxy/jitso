package cli

import "strings"

// RequestHandler runs a request: the whole command line joined by single
// spaces when its first word is not a command and holds whitespace.
type RequestHandler func(ctx *Context, request string) error

var requestHandler RequestHandler

// SetRequestHandler installs the dispatcher (cmd_dispatch.go). The
// dispatcher is not a word of the command table: `jitso dispatch` stays an
// unknown command, as in the oracle.
func SetRequestHandler(h RequestHandler) { requestHandler = h }

// IsRequest reports whether a first word that is not a command opens the
// dispatcher: it holds a character of JavaScript's \s class.
func IsRequest(word string) bool {
	return strings.ContainsFunc(word, isJSSpace)
}

// isJSSpace is the JavaScript \s class (WhiteSpace and LineTerminator). It
// differs from unicode.IsSpace: it has U+FEFF and lacks U+0085.
func isJSSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x00a0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}
