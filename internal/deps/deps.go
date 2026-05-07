package deps

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	defaultGoVersion  = "1.22.5"
	defaultZigVersion = "0.12.0"
	defaultMacSDK     = "11.3"
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
	if root := os.Getenv("SDKROOT"); root != "" {
		if stat, err := os.Stat(root); err == nil && stat.IsDir() {
			return MacSDK{Path: root}, nil
		}
	}

	cacheRoot, err := cacheDir()
	if err != nil {
		return MacSDK{}, err
	}

	installDir := filepath.Join(cacheRoot, "macos-sdk", defaultMacSDK)
	sdkPath := filepath.Join(installDir, "MacOSX"+defaultMacSDK+".sdk")
	if stat, err := os.Stat(sdkPath); err == nil && stat.IsDir() {
		return MacSDK{Path: sdkPath}, nil
	}

	archive, err := downloadMacSDK(ctx, defaultMacSDK, cacheRoot)
	if err != nil {
		return MacSDK{}, err
	}
	if err := extractArchive(archive, installDir); err != nil {
		return MacSDK{}, err
	}
	if stat, err := os.Stat(sdkPath); err == nil && stat.IsDir() {
		return MacSDK{Path: sdkPath}, nil
	}
	return MacSDK{}, errors.New("macos sdk not found after download")
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

	if goos == "darwin" && runtime.GOOS != "darwin" {
		if macSDK == nil {
			sdk, err := EnsureMacSDK(context.Background())
			if err == nil {
				macSDK = &sdk
			}
		}
		if macSDK != nil {
			overrides["SDKROOT"] = macSDK.Path
			overrides["CGO_CFLAGS"] = appendFlag(os.Getenv("CGO_CFLAGS"), "-isysroot", macSDK.Path)
			overrides["CGO_LDFLAGS"] = appendFlag(os.Getenv("CGO_LDFLAGS"), "-isysroot", macSDK.Path)
		}
	}

	return overrides
}

func appendFlag(existing, flag, value string) string {
	if existing == "" {
		return fmt.Sprintf("%s %s", flag, value)
	}
	return fmt.Sprintf("%s %s %s", existing, flag, value)
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
	return fmt.Sprintf("#!/usr/bin/env sh\nexec \"%s\" %s -target %s \"$@\"\n", quoted, mode, zigTarget(goos, goarch))
}
