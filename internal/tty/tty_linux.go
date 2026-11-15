//go:build linux

package tty

import (
	"syscall"
	"unsafe"
)

// isTerminal asks for the terminal attributes of fd (TCGETS): the call
// succeeds only on a terminal.
func isTerminal(fd uintptr) bool {
	var attrs syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TCGETS, uintptr(unsafe.Pointer(&attrs)))
	return errno == 0
}
