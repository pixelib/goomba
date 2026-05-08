package deps

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

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
