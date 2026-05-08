package tui

import (
	"os"

	"golang.org/x/term"
)

func getTerminalSize() (width, height int, err error) {
	return term.GetSize(int(os.Stdout.Fd()))
}
