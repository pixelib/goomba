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

	return fmt.Sprintf(`#!/usr/bin/env sh
# Filter out incompatible flags without mangling the arg list.
filtered=""
for arg in "$@"; do
	if [ "$arg" = "-Wl,--compress-debug-sections=zlib" ]; then
		continue
	fi
	filtered="${filtered}
$arg"
done

old_ifs=$IFS
IFS='
'
set -f
set -- $filtered
set +f
IFS=$old_ifs

exec "%s" %s -target %s "$@"
`, quoted, mode, target)
}
