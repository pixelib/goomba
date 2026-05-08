package embeddedsk

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

// Data will be populated by either embed_darwin.go or embed_stub.go
var Data embed.FS

// IsAvailable returns true if the SDK was actually embedded in this build
func IsAvailable() bool {
	_, err := Data.ReadFile("sdk_marker.txt")
	return err == nil
}

// Extract extracts the embedded SDK to a destination folder.
// It skips symlinks as requested to keep the size down.
func Extract(dest string) error {
	return fs.WalkDir(Data, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == "." || path == "sdk_marker.txt" {
			return err
		}

		// Calculate host path
		// We expect the SDK to be under a top-level 'sdk' folder in the embed
		targetPath := filepath.Join(dest, path)

		if d.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		// Read and write the file
		content, err := Data.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(targetPath, content, 0644)
	})
}
