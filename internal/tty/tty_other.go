//go:build !darwin && !linux

package tty

// isTerminal is false on an OS without a termios ioctl here: a command that
// needs a person at a terminal then takes its no-terminal path.
func isTerminal(fd uintptr) bool { return false }
