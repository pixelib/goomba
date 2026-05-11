package cli

import (
	"fmt"
	"goomba/internal/libver"
	"os"
)

func Run(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printUsage(os.Stdout)
		return 0
	}

	// implement --version, get from build tag if present
	if args[0] == "--version" || (len(args) > 1 && args[1] == "--version") {
		if libver.HasVersion() {
			fmt.Println(libver.GetVersion())
		} else {
			fmt.Println("(version unknown)")
		}
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
