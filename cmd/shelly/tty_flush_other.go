//go:build !(darwin || dragonfly || freebsd || netbsd || openbsd)

package main

// flushStdinBuffer is a no-op on platforms without TIOCFLUSH.
func flushStdinBuffer() {}
