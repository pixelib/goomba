package build

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
