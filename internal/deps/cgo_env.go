package deps

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

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

	if goos == "darwin" {
		if macSDK == nil {
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

			ldflags = appendFlag(ldflags, "-Wl,-headerpad,1144", "")

			overrides["CGO_CFLAGS"] = cflags
			overrides["CGO_LDFLAGS"] = ldflags
			overrides["MACOSX_DEPLOYMENT_TARGET"] = "11.0"

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
