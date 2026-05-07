package cli

import (
	"fmt"
	"os"
)

func Run(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printUsage(os.Stdout)
		return 0
	}

	switch args[0] {
	case "build":
		return runBuild(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		printUsage(os.Stderr)
		return 2
	}
}
