package util

import (
	"os"
	"path/filepath"
	"strings"
)

// FixMacosSDK repairs common issues in macOS SDKs for cross-compilation
func FixMacosSDK(sdkPath string) error {
	// The function signature must match: func(string, os.FileInfo, error) error
	return filepath.Walk(sdkPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and only process .tbd files
		if !info.IsDir() && filepath.Ext(path) == ".tbd" {
			return patchTBD(path)
		}
		return nil
	})
}

func patchTBD(path string) error {
	input, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	content := string(input)
	// Only write if we actually change something to avoid unnecessary disk I/O
	if strings.Contains(content, "x86_64") && !strings.Contains(content, "arm64") {
		// Replacing 'x86_64' with both ensures the linker knows it's valid for both
		output := strings.ReplaceAll(content, "x86_64", "x86_64, arm64")
		return os.WriteFile(path, []byte(output), 0644)
	}

	return nil
}
