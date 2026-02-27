//go:build !(darwin || dragonfly || freebsd || netbsd || openbsd)

package tty

// FlushStdinBuffer is a no-op on platforms without TIOCFLUSH.
func FlushStdinBuffer() {}
