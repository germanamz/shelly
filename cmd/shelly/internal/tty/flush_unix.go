//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package tty

import (
	"os"

	"golang.org/x/sys/unix"
)

// FlushStdinBuffer discards any data left in stdin by prior terminal queries.
func FlushStdinBuffer() {
	//nolint:gosec // Stdin fd is always a small non-negative int.
	fd := int(os.Stdin.Fd())

	old, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return
	}

	raw := *old
	raw.Lflag &^= unix.ECHO | unix.ICANON
	raw.Cc[unix.VMIN] = 0  // return immediately even if no bytes available
	raw.Cc[unix.VTIME] = 0 // no timeout — pure polling read
	if err := unix.IoctlSetTermios(fd, unix.TIOCSETA, &raw); err != nil {
		return
	}
	defer func() { _ = unix.IoctlSetTermios(fd, unix.TIOCSETA, old) }()

	buf := make([]byte, 256)
	for {
		n, err := unix.Read(fd, buf)
		if n <= 0 || err != nil {
			break
		}
	}
}
