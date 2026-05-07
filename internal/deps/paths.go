package deps

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

func cacheDir() (string, error) {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		path := filepath.Join(home, ".goomba", "cache")
		if err := os.MkdirAll(path, 0o755); err != nil {
			return "", err
		}
		return path, nil
	}

	if tmp := os.TempDir(); tmp != "" {
		path := filepath.Join(tmp, "goomba-cache")
		if err := os.MkdirAll(path, 0o755); err != nil {
			return "", err
		}
		return path, nil
	}
	return "", errors.New("no cache directory available")
}

func prependPath(path, existing string) string {
	if existing == "" {
		return path
	}
	return path + string(os.PathListSeparator) + existing
}

func findZigBin(root string) (string, error) {
	var match string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if runtime.GOOS == "windows" {
			if name == "zig.exe" {
				match = path
				return filepath.SkipDir
			}
			return nil
		}
		if name == "zig" {
			match = path
			return filepath.SkipDir
		}
		return nil
	})
	if match == "" {
		return "", errors.New("zig binary not found")
	}
	return match, nil
}
