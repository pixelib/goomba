package deps

import (
	"context"
	"errors"
	"fmt"
	"goomba/internal/embeddedsk"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	defaultGoVersion  = "1.22.5"
	defaultZigVersion = "0.15.0"
)

type Requirements struct {
	NeedZig    bool
	NeedMacSDK bool
	CgoEnabled bool
}

func (r Requirements) Any() bool {
	return r.NeedZig || r.NeedMacSDK
}

func (r Requirements) Count() int {
	count := 0
	if r.NeedZig {
		count++
	}
	if r.NeedMacSDK {
		count++
	}
	return count
}

type GoTool struct {
	Bin  string
	Root string
}

func (g GoTool) EnvOverrides() map[string]string {
	overrides := map[string]string{}
	if g.Root != "" {
		overrides["GOROOT"] = g.Root
		overrides["PATH"] = prependPath(filepath.Join(g.Root, "bin"), os.Getenv("PATH"))
	}
	return overrides
}

func EnsureGo(ctx context.Context, version string) (GoTool, error) {
	if version == "" {
		version = defaultGoVersion
	}
	if root := os.Getenv("GOROOT"); root != "" {
		bin := filepath.Join(root, "bin", "go")
		if _, err := os.Stat(bin); err == nil {
			return GoTool{Bin: bin, Root: root}, nil
		}
	}
	if bin, err := exec.LookPath("go"); err == nil {
		return GoTool{Bin: bin}, nil
	}

	cacheRoot, err := cacheDir()
	if err != nil {
		return GoTool{}, err
	}

	installDir := filepath.Join(cacheRoot, "go", version)
	binPath := filepath.Join(installDir, "go", "bin", "go")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	if _, err := os.Stat(binPath); err == nil {
		return GoTool{Bin: binPath, Root: filepath.Join(installDir, "go")}, nil
	}

	archive, err := downloadGo(ctx, version, cacheRoot)
	if err != nil {
		return GoTool{}, err
	}

	if err := extractArchive(archive, installDir); err != nil {
		return GoTool{}, err
	}

	if _, err := os.Stat(binPath); err != nil {
		return GoTool{}, err
	}
	return GoTool{Bin: binPath, Root: filepath.Join(installDir, "go")}, nil
}

type ZigTool struct {
	Bin string
}

func EnsureZig(ctx context.Context) (ZigTool, error) {
	if bin, err := exec.LookPath("zig"); err == nil {
		return ZigTool{Bin: bin}, nil
	}

	cacheRoot, err := cacheDir()
	if err != nil {
		return ZigTool{}, err
	}

	installDir := filepath.Join(cacheRoot, "zig", defaultZigVersion)
	binPath, err := findZigBin(installDir)
	if err == nil {
		return ZigTool{Bin: binPath}, nil
	}

	archive, err := downloadZig(ctx, defaultZigVersion, cacheRoot)
	if err != nil {
		return ZigTool{}, err
	}

	if err := extractArchive(archive, installDir); err != nil {
		return ZigTool{}, err
	}

	binPath, err = findZigBin(installDir)
	if err != nil {
		return ZigTool{}, err
	}

	return ZigTool{Bin: binPath}, nil
}

type MacSDK struct {
	Path string
}

func EnsureMacSDK(ctx context.Context) (MacSDK, error) {
	cacheRoot, err := cacheDir()
	if err != nil {
		return MacSDK{}, err
	}
	installDir := filepath.Join(cacheRoot, "macos-sdk")

	// 1. Extract if missing
	if _, err := os.Stat(installDir); os.IsNotExist(err) {
		if !embeddedsk.IsAvailable() {
			return MacSDK{}, fmt.Errorf("this build of goomba doesn't support macOS targets...")
		}
		fmt.Println(">> Extracting embedded Apple SDK...")
		if err := embeddedsk.Extract(installDir); err != nil {
			return MacSDK{}, fmt.Errorf("failed to extract embedded SDK: %w", err)
		}
	}

	return MacSDK{Path: installDir}, nil
}

func HasCCompiler() bool {
	if _, err := exec.LookPath("cc"); err == nil {
		return true
	}
	if _, err := exec.LookPath("gcc"); err == nil {
		return true
	}
	if _, err := exec.LookPath("clang"); err == nil {
		return true
	}
	return false
}

func CgoEnv(goos, goarch string, zigTool *ZigTool, macSDK *MacSDK) map[string]string {
	if os.Getenv("CGO_ENABLED") == "0" {
		return nil
	}

	reqZig := goos != runtime.GOOS || goarch != runtime.GOARCH || !HasCCompiler()
	if !reqZig && goos == runtime.GOOS && goarch == runtime.GOARCH {
		return nil
	}
	if zigTool == nil {
		tool, err := EnsureZig(context.Background())
		if err != nil {
			return nil
		}
		zigTool = &tool
	}
	ccPath, cxxPath := zigWrapperPaths(zigTool.Bin, goos, goarch)

	overrides := map[string]string{
		"CC":  zigCc(zigTool.Bin, goos, goarch),
		"CXX": zigCxx(zigTool.Bin, goos, goarch),
	}
	if ccPath != "" {
		overrides["CC"] = ccPath
	}
	if cxxPath != "" {
		overrides["CXX"] = cxxPath
	}
	if cacheRoot, err := cacheDir(); err == nil {
		zigCache := filepath.Join(cacheRoot, "zig-cache")
		_ = os.MkdirAll(zigCache, 0o755)
		overrides["ZIG_GLOBAL_CACHE_DIR"] = zigCache
	}

	//if goos == "darwin" && runtime.GOOS != "darwin" {
	//	if macSDK == nil {
	//		sdk, err := EnsureMacSDK(context.Background())
	//		if err == nil {
	//			macSDK = &sdk
	//		}
	//	}
	//	if macSDK != nil {
	//		frameworkPath := filepath.Join(macSDK.Path, "System", "Library", "Frameworks")
	//		libPath := filepath.Join(macSDK.Path, "usr", "lib")
	//
	//		overrides["SDKROOT"] = macSDK.Path
	//
	//		// CFLAGS: Tell the compiler where headers and framework headers live
	//		cflags := appendFlag(os.Getenv("CGO_CFLAGS"), "-isysroot", macSDK.Path)
	//		cflags = appendFlag(cflags, "-iframework", frameworkPath)
	//
	//		// LDFLAGS: This is where the magic happens
	//		ldflags := appendFlag(os.Getenv("CGO_LDFLAGS"), "-isysroot", macSDK.Path)
	//		ldflags = appendFlag(ldflags, "-F", frameworkPath)
	//		ldflags = appendFlag(ldflags, "-L", libPath)
	//
	//		// FORCE external linking mode so Zig handles the final Mach-O creation
	//		// and add -headerpad for Go's internal linker requirements
	//		//ldflags = appendFlag(ldflags, "-linkmode", "external")
	//
	//		//
	//		ldflags = appendFlag(ldflags, "-s", "")
	//		ldflags = appendFlag(ldflags, "-w", "")
	//
	//		overrides["CGO_CFLAGS"] = cflags
	//		overrides["CGO_LDFLAGS"] = ldflags
	//
	//		// Target 11.0 to ensure TBD v4 compatibility in Zig
	//		overrides["MACOSX_DEPLOYMENT_TARGET"] = "11.0"
	//	}
	//}
	if goos == "darwin" {
		if macSDK == nil {
			// Prefer real SDK from xcrun first
			sdk, err := EnsureMacSDK(context.Background())
			if err == nil {
				macSDK = &sdk
			}
		}

		if macSDK != nil {
			frameworkPath := filepath.Join(macSDK.Path, "System", "Library", "Frameworks")
			libPath := filepath.Join(macSDK.Path, "usr", "lib")

			overrides["SDKROOT"] = macSDK.Path

			cflags := os.Getenv("CGO_CFLAGS")
			cflags = appendFlag(cflags, "-isysroot", macSDK.Path)
			cflags = appendFlag(cflags, "-iframework", frameworkPath)

			ldflags := os.Getenv("CGO_LDFLAGS")
			ldflags = appendFlag(ldflags, "-isysroot", macSDK.Path)
			ldflags = appendFlag(ldflags, "-F", frameworkPath)
			ldflags = appendFlag(ldflags, "-L", libPath)

			// Fix 1: Ensure Mach-O headers have enough padding for Apple tools
			ldflags = appendFlag(ldflags, "-Wl,-headerpad,1144", "")

			overrides["CGO_CFLAGS"] = cflags
			overrides["CGO_LDFLAGS"] = ldflags
			overrides["MACOSX_DEPLOYMENT_TARGET"] = "11.0"

			// Fix 2: Force Go to skip the strip phase to avoid the __LLVM segment error
			// We wrap the flags in quotes so GOFLAGS parses it as a single argument
			existingGoFlags := os.Getenv("GOFLAGS")
			overrides["GOFLAGS"] = appendFlag(existingGoFlags, "-ldflags=-s -w", "")
		}
	}

	return overrides
}

func appendFlag(existing, flag, value string) string {
	newPart := flag
	if value != "" {
		newPart += " " + value
	}
	if existing == "" {
		return newPart
	}
	return existing + " " + newPart
}

func zigWrapperPaths(zigPath, goos, goarch string) (string, string) {
	cacheRoot, err := cacheDir()
	if err != nil {
		return "", ""
	}
	wrapperDir := filepath.Join(cacheRoot, "zig-wrappers")
	if err := os.MkdirAll(wrapperDir, 0o755); err != nil {
		return "", ""
	}

	ccPath := filepath.Join(wrapperDir, fmt.Sprintf("cc-%s-%s", goos, goarch))
	cxxPath := filepath.Join(wrapperDir, fmt.Sprintf("cxx-%s-%s", goos, goarch))
	if runtime.GOOS == "windows" {
		ccPath += ".cmd"
		cxxPath += ".cmd"
	}

	if err := writeZigWrapper(ccPath, zigPath, "cc", goos, goarch); err != nil {
		ccPath = ""
	}
	if err := writeZigWrapper(cxxPath, zigPath, "c++", goos, goarch); err != nil {
		cxxPath = ""
	}

	return ccPath, cxxPath
}

func writeZigWrapper(path, zigPath, mode, goos, goarch string) error {
	content := zigWrapperScript(zigPath, mode, goos, goarch)
	if content == "" {
		return errors.New("unsupported wrapper")
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return err
	}
	return nil
}

func zigWrapperScript(zigPath, mode, goos, goarch string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("@echo off\r\n\"%s\" %s -target %s %%*\r\n", zigPath, mode, zigTarget(goos, goarch))
	}

	quoted := strings.ReplaceAll(zigPath, "\"", "\\\"")
	target := zigTarget(goos, goarch)

	// Using a proper array-based approach in shell to filter flags
	return fmt.Sprintf(`#!/usr/bin/env sh
# Filter out incompatible flags
args=""
for arg in "$@"; do
  if [ "$arg" = "-Wl,--compress-debug-sections=zlib" ]; then
    continue
  fi
  # This builds a list of arguments properly handled by 'set'
  set -- "$@" "$arg"
  shift
done

exec "%s" %s -target %s "$@"
`, quoted, mode, target)
}
