package cli

import (
	"fmt"
	"io"
)

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "goomba is a go build helper")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  goomba build [flags] [-- <go build args>]")
	fmt.Fprintln(w, "  goomba help")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "commands:")
	fmt.Fprintln(w, "  build")
	fmt.Fprintln(w, "  help")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "flags:")
	fmt.Fprintln(w, "  --platforms   comma separated list: linux, macos, windows")
	fmt.Fprintln(w, "  --arch        comma separated list: amd64, arm64, x64")
	fmt.Fprintln(w, "  --no-parallel run builds one by one")
	fmt.Fprintln(w, "  --no-tui      disable tui progress")
	fmt.Fprintln(w, "  --no-validation skip validation step")
	fmt.Fprintln(w, "  --strict      fail if any target fails and remove dist output")
	fmt.Fprintln(w, "  --verbose     enable verbose logging")
	fmt.Fprintln(w, "  --java-home   override JAVA_HOME for JNI includes")
	fmt.Fprintln(w, "  --go-args     append args to go build, repeatable")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "examples:")
	fmt.Fprintln(w, "  goomba build --platforms linux,macos,windows")
	fmt.Fprintln(w, "  goomba build --platforms macos --arch x64,arm64")
	fmt.Fprintln(w, "  goomba build -- --buildmode=c-shared -o out/lib.so")
}

func printBuildUsage(w io.Writer) {
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  goomba build [flags] [-- <go build args>]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "flags:")
	fmt.Fprintln(w, "  --platforms   comma separated list: linux, macos, windows")
	fmt.Fprintln(w, "  --arch        comma separated list: amd64, arm64, x64")
	fmt.Fprintln(w, "  --no-parallel run builds one by one")
	fmt.Fprintln(w, "  --no-tui      disable tui progress")
	fmt.Fprintln(w, "  --no-validation skip validation step")
	fmt.Fprintln(w, "  --strict      fail if any target fails and remove dist output")
	fmt.Fprintln(w, "  --verbose     enable verbose logging")
	fmt.Fprintln(w, "  --java-home   override JAVA_HOME for JNI includes")
	fmt.Fprintln(w, "  --go-args     append args to go build, repeatable")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "examples:")
	fmt.Fprintln(w, "  goomba build --platforms linux,macos,windows")
	fmt.Fprintln(w, "  goomba build --platforms macos --arch x64,arm64")
	fmt.Fprintln(w, "  goomba build -- --buildmode=c-shared -o out/lib.so")
}
