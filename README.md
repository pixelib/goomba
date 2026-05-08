# goomba

goomba is a go build helper that runs build matrices with one command and downloads missing dependencies on demand.

The goal is a one step build process for all platforms without requiring local toolchains. It should be easy to run locally or in CI for production cross platform builds without Docker, macOS hosts, or a preinstalled Go toolchain.

## goals

- single binary, no required system tools besides what goomba downloads
- build matrix support for platforms and architectures
- optional tui progress and parallel builds
- predictable output layout under ./dist

## install

Use your usual go install flow from this repo.

## usage

```
goomba build [flags] [-- <go build args>]
```

Help is available with:

```
goomba --help
goomba help
goomba build --help
```

### examples

```
goomba build --platforms linux,macos,windows
```

```
goomba build --platforms macos --arch x64,arm64
```

```
goomba build -- --buildmode=c-shared -o dist/libshared.so
```

### flags

- --platforms: comma separated list of linux, macos, windows
- --arch: comma separated list of amd64, arm64, x64
- --no-parallel: run builds one by one
- --no-tui: disable tui progress output
- --no-validation: skip validation step
- --strict: fail if any target fails and remove dist output
- --verbose: enable verbose logging
- --java-home: override JAVA_HOME for JNI includes
- --go-args: append go build args, repeatable

By default, failed targets are logged and skipped while the rest continue.

When the tui is enabled, go command output is shown as a temporary, last-10-line log under each progress bar.

With --verbose, goomba keeps a longer log window and prints command and env details per step.

## output layout

Artifacts are placed in:

```
./dist/<platform>/<arch>/<binary>
```

Platform uses macos for darwin.

## phases

1. preparing golang
2. validation
3. downloading dependencies
4. build

## dependency handling

- respects host env vars for GOROOT, GOPATH, CGO_ENABLED, CGO_CFLAGS, GO111MODULE, GOWORK, GOPROXY
- if go is missing, goomba downloads it into ~/.goomba/cache or /tmp
- if a c compiler is missing and cgo is enabled, goomba downloads zig and uses it
- macos cross builds on non macos hosts pull an sdk from https://github.com/phracker/MacOSX-SDKs

## technical notes

goomba respects these go env variables when set on the host:

- GOROOT
- GOPATH
- CGO_ENABLED
- CGO_CFLAGS
- GO111MODULE
- GOWORK
- GOPROXY

goomba also injects these placeholders for per-target values:

- ${GOOMBA_OS} (go os, for example darwin)
- ${GOOMBA_ARCH} (go arch, for example arm64)
- ${GOOMBA_PLATFORM} (display platform, for example macos)

If JNI headers are needed, set JAVA_HOME or use --java-home to point at a JDK.

## why not goreleaser

goreleaser is focused on release automation, packaging, and publishing. goomba focuses on a fast build matrix with zero local toolchain requirements and no release config. If you only need cross platform builds from any go project root, goomba aims to be lighter weight.

## test projects

Test projects live in _testproject and are used by integration tests.

- _testproject/hello: pure go sample
- _testproject/cgo: cgo sample

## development

Run unit and integration tests:

```
go test ./...
```

To skip integration tests:

```
go test -short ./...
```
