package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuildHello(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoRoot := findRepoRoot(t)
	workDir := filepath.Join(repoRoot, "_testproject", "hello")
	output := runGoombaBuild(t, repoRoot, workDir, []string{"build", "--no-parallel", "--no-tui"}, false)
	verifyHelloOutput(t, workDir)
	if output != "" {
		t.Fatalf("unexpected output captured in no-tui mode")
	}
}

func TestBuildHelloTui(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoRoot := findRepoRoot(t)
	workDir := filepath.Join(repoRoot, "_testproject", "hello")
	output := runGoombaBuild(t, repoRoot, workDir, []string{"build", "--no-parallel"}, true)
	verifyHelloOutput(t, workDir)

	label := runtime.GOOS
	if label == "darwin" {
		label = "macos"
	}
	if !strings.Contains(output, "preparing golang") {
		t.Fatalf("expected tui output to include preparing golang")
	}
	if !strings.Contains(output, "validation") {
		t.Fatalf("expected tui output to include validation")
	}
	if !strings.Contains(output, label+"-"+runtime.GOARCH) {
		t.Fatalf("expected tui output to include target label")
	}
}

func runGoombaBuild(t *testing.T, repoRoot, workDir string, args []string, capture bool) string {
	cmdArgs := append([]string{"run", filepath.Join(repoRoot, "cmd", "goomba")}, args...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	var buf bytes.Buffer
	if capture {
		cmd.Stdout = &buf
		cmd.Stderr = &buf
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		t.Fatalf("goomba build failed: %v", err)
	}

	return buf.String()
}

func verifyHelloOutput(t *testing.T, workDir string) {
	label := runtime.GOOS
	if label == "darwin" {
		label = "macos"
	}
	binName := filepath.Base(workDir)
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}

	outPath := filepath.Join(workDir, "dist", label, runtime.GOARCH, binName)
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected output at %s", outPath)
	}

	_ = os.RemoveAll(filepath.Join(workDir, "dist"))
}

func findRepoRoot(t *testing.T) string {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	path := wd
	for {
		if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
			return path
		}
		next := filepath.Dir(path)
		if next == path {
			break
		}
		path = next
	}
	t.Fatalf("go.mod not found from %s", wd)
	return ""
}
