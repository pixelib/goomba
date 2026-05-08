package build

import "testing"

func TestParsePlatforms(t *testing.T) {
	plats, err := ParsePlatforms([]string{"linux", "macos", "windows", "darwin", "win"})
	if err != nil {
		t.Fatalf("parse platforms: %v", err)
	}
	if len(plats) != 3 {
		t.Fatalf("expected 3 platforms, got %d", len(plats))
	}
}

func TestParseArchs(t *testing.T) {
	arches, err := ParseArchs([]string{"x64", "amd64", "arm64"})
	if err != nil {
		t.Fatalf("parse archs: %v", err)
	}
	if len(arches) != 2 {
		t.Fatalf("expected 2 archs, got %d", len(arches))
	}
}

func TestBuildMatrix(t *testing.T) {
	targets, err := BuildMatrix([]string{"linux", "macos"}, []string{"amd64"})
	if err != nil {
		t.Fatalf("build matrix: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
}
