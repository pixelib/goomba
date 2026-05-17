package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestExpandPlaceholders(t *testing.T) {
	target := Target{GOOS: "linux", GOARCH: "amd64", Label: "linux"}
	value := "${GOOMBA_OS}/${GOOMBA_ARCH}/${GOOMBA_PLATFORM}"
	got := expandPlaceholders(value, target)
	if got != "linux/amd64/linux" {
		t.Fatalf("unexpected placeholders: %s", got)
	}
}

func TestExpandArgs(t *testing.T) {
	target := Target{GOOS: "darwin", GOARCH: "arm64", Label: "macos"}
	args := []string{"-ldflags=${GOOMBA_PLATFORM}", "-tags", "${GOOMBA_OS}_${GOOMBA_ARCH}"}
	got := expandArgs(args, target)
	want := []string{"-ldflags=macos", "-tags", "darwin_arm64"}
	if len(got) != len(want) {
		t.Fatalf("expected %d args, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg %d: expected %q, got %q", i, want[i], got[i])
		}
	}
}

func TestJniOsInclude(t *testing.T) {
	cases := map[string]string{
		"darwin":  "darwin",
		"windows": "win32",
		"linux":   "linux",
		"plan9":   "",
	}
	for goos, want := range cases {
		if got := jniOsInclude(goos); got != want {
			t.Fatalf("goos %s: expected %q, got %q", goos, want, got)
		}
	}
}

func TestAppendCFlags(t *testing.T) {
	got := appendCFlags("-O2  ", "/a", "/b")
	want := "-O2 -I/a -I/b"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}

	got = appendCFlags("", "/a")
	want = "-I/a"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestEnvLookupPickEnv(t *testing.T) {
	entries := []string{"FOO=bar", "INVALID", "BAZ=qux"}
	if val, ok := envLookup(entries, "FOO"); !ok || val != "bar" {
		t.Fatalf("expected FOO=bar, got %q (ok=%v)", val, ok)
	}
	if _, ok := envLookup(entries, "MISSING"); ok {
		t.Fatalf("expected missing key to return ok=false")
	}

	picked := pickEnv(entries, []string{"BAZ", "MISSING"})
	if len(picked) != 1 || picked[0] != "BAZ=qux" {
		t.Fatalf("unexpected picked env: %v", picked)
	}

	if pickEnv(nil, []string{"FOO"}) != nil {
		t.Fatalf("expected nil when entries are empty")
	}
	if pickEnv(entries, nil) != nil {
		t.Fatalf("expected nil when keys are empty")
	}
}

func TestJniCgoEnv(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		env := []string{"CGO_ENABLED=0", "JAVA_HOME=/tmp/nowhere"}
		if got := jniCgoEnv("linux", env); got != nil {
			t.Fatalf("expected nil when CGO is disabled, got %v", got)
		}
	})

	t.Run("missing-include", func(t *testing.T) {
		javaHome := t.TempDir()
		env := []string{"JAVA_HOME=" + javaHome}
		if got := jniCgoEnv("linux", env); got != nil {
			t.Fatalf("expected nil when include dir missing, got %v", got)
		}
	})

	t.Run("valid", func(t *testing.T) {
		javaHome := t.TempDir()
		includeDir := filepath.Join(javaHome, "include")
		osIncludeDir := filepath.Join(includeDir, "linux")
		if err := os.MkdirAll(osIncludeDir, 0o755); err != nil {
			t.Fatalf("mkdir include: %v", err)
		}
		env := []string{"JAVA_HOME=" + javaHome, "CGO_CFLAGS=-O2"}
		got := jniCgoEnv("linux", env)
		if got == nil {
			t.Fatalf("expected CGO flags, got nil")
		}
		want := "-O2 -I" + includeDir + " -I" + osIncludeDir
		if got["CGO_CFLAGS"] != want {
			t.Fatalf("expected %q, got %q", want, got["CGO_CFLAGS"])
		}
	})
}

func TestHandleBuildErrorsStrict(t *testing.T) {
	workDir := t.TempDir()
	distDir := filepath.Join(workDir, "dist")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatalf("mkdir dist: %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "marker.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write dist marker: %v", err)
	}

	cfg := Config{WorkDir: workDir, Strict: true}
	err := handleBuildErrors(cfg, []string{"boom"})
	if err == nil {
		t.Fatalf("expected error in strict mode")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected error to include build errors, got %v", err)
	}
	if _, statErr := os.Stat(distDir); !os.IsNotExist(statErr) {
		t.Fatalf("expected dist dir removed, stat err: %v", statErr)
	}
}

func TestHandleBuildErrorsNonStrict(t *testing.T) {
	cfg := Config{WorkDir: t.TempDir(), Strict: false}
	if err := handleBuildErrors(cfg, []string{"boom"}); err != nil {
		t.Fatalf("expected nil error in non-strict mode, got %v", err)
	}
}

func TestParsePlatformsDefaultsToRuntime(t *testing.T) {
	plats, err := ParsePlatforms(nil)
	if err != nil {
		t.Fatalf("parse platforms: %v", err)
	}
	if len(plats) != 1 || plats[0] != runtime.GOOS {
		t.Fatalf("expected runtime platform %s, got %v", runtime.GOOS, plats)
	}
}

func TestParseArchsDefaultsToRuntime(t *testing.T) {
	arches, err := ParseArchs(nil)
	if err != nil {
		t.Fatalf("parse archs: %v", err)
	}
	if len(arches) != 1 || arches[0] != runtime.GOARCH {
		t.Fatalf("expected runtime arch %s, got %v", runtime.GOARCH, arches)
	}
}

func TestBuildMatrixDarwinLabel(t *testing.T) {
	targets, err := BuildMatrix([]string{"darwin"}, []string{"amd64"})
	if err != nil {
		t.Fatalf("build matrix: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Label != "macos" {
		t.Fatalf("expected label macos, got %s", targets[0].Label)
	}
}

func TestBuildMatrixOrdering(t *testing.T) {
	targets, err := BuildMatrix([]string{"windows", "linux"}, []string{"arm64", "amd64"})
	if err != nil {
		t.Fatalf("build matrix: %v", err)
	}
	got := make([]string, 0, len(targets))
	for _, target := range targets {
		got = append(got, target.GOOS+"/"+target.GOARCH)
	}
	want := []string{
		"linux/amd64",
		"linux/arm64",
		"windows/amd64",
		"windows/arm64",
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d targets, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("target %d: expected %q, got %q", i, want[i], got[i])
		}
	}
}

func TestBuildManifest(t *testing.T) {
	workDir := t.TempDir()
	cfg := Config{WorkDir: workDir, OutputBase: ""}
	baseDir := outputBaseDir(cfg)

	targets := []Target{
		{GOOS: "linux", GOARCH: "amd64", Label: "linux"},
		{GOOS: "darwin", GOARCH: "arm64", Label: "macos"},
	}

	// Create artifact files
	for _, target := range targets {
		dir := filepath.Join(baseDir, target.Label, target.GOARCH)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		binName := "app"
		if target.GOOS == "windows" {
			binName += ".exe"
		}
		content := "binary content for " + target.GOOS + "/" + target.GOARCH
		if err := os.WriteFile(filepath.Join(dir, binName), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	m := BuildManifest(cfg, targets, nil, 5*time.Second)
	if m == nil {
		t.Fatal("expected non-nil manifest")
	}
	if m.Targets != 2 {
		t.Fatalf("expected 2 targets, got %d", m.Targets)
	}
	if m.Succeeded != 2 {
		t.Fatalf("expected 2 succeeded, got %d", m.Succeeded)
	}
	if m.Failed != 0 {
		t.Fatalf("expected 0 failed, got %d", m.Failed)
	}
	if m.Duration != "5s" {
		t.Fatalf("expected duration 5s, got %s", m.Duration)
	}
	if m.GeneratedAt == "" {
		t.Fatal("expected non-empty generated_at")
	}
	if len(m.Artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(m.Artifacts))
	}

	for _, a := range m.Artifacts {
		if a.SHA256 == "" {
			t.Fatalf("expected non-empty sha256 for %s", a.Path)
		}
		if a.Size <= 0 {
			t.Fatalf("expected positive size for %s", a.Path)
		}
	}

	// Check paths
	paths := make([]string, len(m.Artifacts))
	for i, a := range m.Artifacts {
		paths[i] = a.Path
	}
	if paths[0] != "linux/amd64/app" || paths[1] != "macos/arm64/app" {
		t.Fatalf("unexpected paths: %v", paths)
	}
}

func TestBuildManifestEmpty(t *testing.T) {
	workDir := t.TempDir()
	cfg := Config{WorkDir: workDir, OutputBase: ""}
	m := BuildManifest(cfg, nil, nil, 0)
	if m == nil {
		t.Fatal("expected non-nil manifest")
	}
	if len(m.Artifacts) != 0 {
		t.Fatalf("expected 0 artifacts, got %d", len(m.Artifacts))
	}
	if m.Duration != "0s" {
		t.Fatalf("expected duration 0s, got %s", m.Duration)
	}
}

func TestBuildManifestSkipsManifestFile(t *testing.T) {
	workDir := t.TempDir()
	cfg := Config{WorkDir: workDir, OutputBase: "out"}
	baseDir := outputBaseDir(cfg)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write a real-looking artifact
	os.MkdirAll(filepath.Join(baseDir, "linux", "amd64"), 0o755)
	os.WriteFile(filepath.Join(baseDir, "linux", "amd64", "app"), []byte("data"), 0o644)
	// Write a pre-existing manifest file
	os.WriteFile(filepath.Join(baseDir, "builds.manifest"), []byte(`{"old":true}`), 0o644)

	m := BuildManifest(cfg, []Target{{GOOS: "linux", GOARCH: "amd64", Label: "linux"}}, nil, 0)
	if len(m.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact (manifest excluded), got %d", len(m.Artifacts))
	}
	if m.Artifacts[0].Path != "linux/amd64/app" {
		t.Fatalf("unexpected artifact path: %s", m.Artifacts[0].Path)
	}
}

func TestWriteManifest(t *testing.T) {
	workDir := t.TempDir()
	m := &Manifest{
		GeneratedAt: "2026-01-01T00:00:00Z",
		Targets:     1,
		Succeeded:   1,
		Failed:      0,
		Duration:    "1s",
		Artifacts: []ManifestEntry{
			{Path: "linux/amd64/app", Platform: "linux", Arch: "amd64", Size: 100, SHA256: "abcdef"},
		},
	}

	if err := WriteManifest(workDir, m); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	manifestPath := filepath.Join(workDir, "builds.manifest")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var decoded Manifest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if decoded.Targets != 1 {
		t.Fatalf("expected 1 target, got %d", decoded.Targets)
	}
	if len(decoded.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(decoded.Artifacts))
	}
	if decoded.Artifacts[0].Path != "linux/amd64/app" {
		t.Fatalf("unexpected path: %s", decoded.Artifacts[0].Path)
	}
	if decoded.Artifacts[0].SHA256 != "abcdef" {
		t.Fatalf("unexpected sha256: %s", decoded.Artifacts[0].SHA256)
	}
}

func TestFormatSize(t *testing.T) {
	cases := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024*1024 + 512*1024, "1.5 MB"},
	}
	for _, c := range cases {
		got := formatSize(c.bytes)
		if got != c.want {
			t.Fatalf("formatSize(%d): expected %q, got %q", c.bytes, c.want, got)
		}
	}
}

func TestFormatSHA256Short(t *testing.T) {
	if got := formatSHA256Short(""); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := formatSHA256Short("abc"); got != "abc" {
		t.Fatalf("expected abc, got %q", got)
	}
	full := "abcdef1234567890abcdef1234567890"
	if got := formatSHA256Short(full); got != "abcdef123456" {
		t.Fatalf("expected abcdef123456, got %q", got)
	}
}

func TestManifestTotalSize(t *testing.T) {
	m := &Manifest{
		Artifacts: []ManifestEntry{
			{Size: 1000},
			{Size: 2000},
			{Size: 3000},
		},
	}
	if total := m.TotalSize(); total != 6000 {
		t.Fatalf("expected 6000, got %d", total)
	}
}

func TestBuildManifestWithFailedTargets(t *testing.T) {
	workDir := t.TempDir()
	cfg := Config{WorkDir: workDir, OutputBase: ""}
	baseDir := outputBaseDir(cfg)

	targets := []Target{
		{GOOS: "linux", GOARCH: "amd64", Label: "linux"},
		{GOOS: "darwin", GOARCH: "arm64", Label: "macos"},
	}

	// Only create first artifact (simulating partial failure)
	dir := filepath.Join(baseDir, "linux", "amd64")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "app"), []byte("data"), 0o644)

	// With 1 error
	m := BuildManifest(cfg, targets, []string{"macos/arm64: build failed"}, time.Second)
	if m.Targets != 2 {
		t.Fatalf("expected 2 targets, got %d", m.Targets)
	}
	if m.Succeeded != 1 {
		t.Fatalf("expected 1 succeeded, got %d", m.Succeeded)
	}
	if m.Failed != 1 {
		t.Fatalf("expected 1 failed, got %d", m.Failed)
	}
	// Only one artifact should be found
	if len(m.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact (partial build), got %d", len(m.Artifacts))
	}
	if m.Artifacts[0].Path != "linux/amd64/app" {
		t.Fatalf("unexpected artifact path: %s", m.Artifacts[0].Path)
	}
}
