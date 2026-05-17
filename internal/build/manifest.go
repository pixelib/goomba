package build

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ManifestEntry struct {
	Path     string `json:"path"`
	Platform string `json:"platform"`
	Arch     string `json:"arch"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
}

type Manifest struct {
	GeneratedAt string          `json:"generated_at"`
	Targets     int             `json:"targets"`
	Succeeded   int             `json:"succeeded"`
	Failed      int             `json:"failed"`
	Duration    string          `json:"duration"`
	Artifacts   []ManifestEntry `json:"artifacts"`
}

func BuildManifest(cfg Config, targets []Target, errs []string, duration time.Duration) *Manifest {
	baseDir := outputBaseDir(cfg)

	targetDirs := make(map[string]struct{ platform, arch string })
	for _, t := range targets {
		dir := filepath.Join(t.Label, t.GOARCH)
		targetDirs[dir] = struct{ platform, arch string }{t.Label, t.GOARCH}
	}

	m := &Manifest{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Targets:     len(targets),
		Succeeded:   len(targets) - len(errs),
		Failed:      len(errs),
		Duration:    duration.Round(time.Second).String(),
	}

	info, err := os.Stat(baseDir)
	if err != nil {
		return m
	}
	if !info.IsDir() {
		return m
	}

	filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(baseDir, path)
		if err != nil {
			return nil
		}
		if rel == "builds.manifest" {
			return nil
		}

		entry := ManifestEntry{
			Path: filepath.ToSlash(rel),
			Size: info.Size(),
		}

		dir := filepath.Dir(rel)
		if ti, ok := targetDirs[dir]; ok {
			entry.Platform = ti.platform
			entry.Arch = ti.arch
		} else {
			parts := strings.SplitN(rel, string(filepath.Separator), 3)
			if len(parts) >= 2 {
				entry.Platform = parts[0]
				entry.Arch = parts[1]
			}
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return nil
		}
		entry.SHA256 = hex.EncodeToString(h.Sum(nil))

		m.Artifacts = append(m.Artifacts, entry)
		return nil
	})

	sort.Slice(m.Artifacts, func(i, j int) bool {
		return m.Artifacts[i].Path < m.Artifacts[j].Path
	})
	return m
}

func WriteManifest(baseDir string, m *Manifest) error {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(baseDir, "builds.manifest")
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (m *Manifest) TotalSize() int64 {
	var total int64
	for _, a := range m.Artifacts {
		total += a.Size
	}
	return total
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func formatSHA256Short(s string) string {
	if len(s) < 12 {
		return s
	}
	return s[:12]
}
