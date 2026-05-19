//go:build windows

package cmd

import "golang.org/x/term"

func setTerminalRaw(fd int) (*term.State, error) {
	return term.MakeRaw(fd)
}

func restoreTerminal(fd int, oldState *term.State) {
	_ = term.Restore(fd, oldState)
}
