//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// flushStdinBuffer discards any data left in stdin by prior terminal queries.
//
// termenv's readNextResponse() reads OSC responses up to the ESC byte of the
// String Terminator (ESC \) but does NOT consume the trailing '\'. This
// leftover backslash sits in the tty line discipline's canonical buffer and
// cannot be flushed with TIOCFLUSH when ICANON is enabled (canonical mode
// only delivers complete lines).
//
// The fix: temporarily disable ICANON so all pending bytes become readable,
// drain them with non-blocking reads, then restore the terminal.
func flushStdinBuffer() {
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

	// Go's runtime sets fd 0 to non-blocking for its poller, so unix.Read
	// returns EAGAIN immediately when the buffer is empty.
	buf := make([]byte, 256)
	for {
		n, err := unix.Read(fd, buf)
		if n <= 0 || err != nil {
			break
		}
	}
}
