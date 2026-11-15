// Package tty answers one question: is a file a terminal. It asks the kernel
// with the termios ioctl instead of looking at the file mode, because a
// character device is not always a terminal (/dev/null is one, and a step of
// the contract runner is given it as its stdin).
//
// It is standard library only. The ioctl lives in tty_darwin.go and
// tty_linux.go; every other OS answers false (tty_other.go).
package tty

import "os"

// IsTerminal reports whether f is open on a terminal. A nil file, a closed
// file, a pipe, a regular file and a device that is not a terminal are false.
func IsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	// SyscallConn reads the descriptor without changing the file's blocking
	// mode, which Fd would.
	conn, err := f.SyscallConn()
	if err != nil {
		return false
	}
	is := false
	if err := conn.Control(func(fd uintptr) { is = isTerminal(fd) }); err != nil {
		return false
	}
	return is
}
