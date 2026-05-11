package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"goomba/internal/build"
)

type argsList []string

func (a *argsList) String() string {
	return strings.Join(*a, " ")
}

func (a *argsList) Set(value string) error {
	parts, err := splitArgs(value)
	if err != nil {
		return err
	}
	*a = append(*a, parts...)
	return nil
}

func runBuild(args []string) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { printBuildUsage(os.Stderr) }

	var platforms string
	var archs string
	var noParallel bool
	var noTui bool
	var noValidation bool
	var strict bool
	var verbose bool
	var javaHome string
	var outputBase string
	var goArgs argsList
	var cgoEnabled bool

	fs.StringVar(&platforms, "platforms", "", "comma separated platforms")
	fs.StringVar(&archs, "arch", "", "comma separated architectures")
	fs.BoolVar(&noParallel, "no-parallel", false, "disable parallel builds")
	fs.BoolVar(&noTui, "no-tui", false, "disable tui progress")
	fs.BoolVar(&noValidation, "no-validation", false, "skip validation step")
	fs.BoolVar(&strict, "strict", false, "fail if any target fails and remove output directory")
	fs.BoolVar(&verbose, "verbose", false, "enable verbose logging")
	fs.StringVar(&javaHome, "java-home", "", "override JAVA_HOME for JNI includes")
	fs.StringVar(&outputBase, "out", "", "output base directory (default: dist)")
	fs.Var(&goArgs, "go-args", "additional go build arguments, repeatable")
	fs.BoolVar(&cgoEnabled, "cgo-enabled", false, "enable CGO for builds")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	goArgs = append(goArgs, fs.Args()...)

	workDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read working directory: %v\n", err)
		return 1
	}

	cfg := build.Config{
		WorkDir:      workDir,
		OutputBase:   outputBase,
		Platforms:    splitList(platforms),
		Archs:        splitList(archs),
		NoParallel:   noParallel,
		NoTui:        noTui,
		NoValidation: noValidation,
		Strict:       strict,
		Verbose:      verbose,
		JavaHome:     javaHome,
		GoArgs:       []string(goArgs),
		ValidateCmd:  []string{"go", "vet", "./..."},
		GoVersion:    "1.22.5",
		CgoEnabled:   cgoEnabled,
	}

	if err := build.Run(context.Background(), cfg); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n", err)
		return 1
	}

	return 0
}
