package main

import (
	"os"

	"goomba/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
