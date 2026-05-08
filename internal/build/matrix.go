package build

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
)

type Target struct {
	GOOS  string
	GOARCH string
	Label string
}

func ParsePlatforms(items []string) ([]string, error) {
	if len(items) == 0 {
		return []string{runtime.GOOS}, nil
	}

	alias := map[string]string{
		"macos":  "darwin",
		"darwin": "darwin",
		"linux":  "linux",
		"windows": "windows",
		"win":    "windows",
	}

	seen := map[string]bool{}
	var out []string
	for _, item := range items {
		key := strings.ToLower(strings.TrimSpace(item))
		val, ok := alias[key]
		if !ok {
			return nil, fmt.Errorf("unknown platform: %s", item)
		}
		if !seen[val] {
			seen[val] = true
			out = append(out, val)
		}
	}
	sort.Strings(out)
	return out, nil
}

func ParseArchs(items []string) ([]string, error) {
	if len(items) == 0 {
		return []string{runtime.GOARCH}, nil
	}
	alias := map[string]string{
		"amd64": "amd64",
		"x64":   "amd64",
		"x86_64": "amd64",
		"arm64": "arm64",
		"aarch64": "arm64",
	}
	seen := map[string]bool{}
	var out []string
	for _, item := range items {
		key := strings.ToLower(strings.TrimSpace(item))
		val, ok := alias[key]
		if !ok {
			return nil, fmt.Errorf("unknown arch: %s", item)
		}
		if !seen[val] {
			seen[val] = true
			out = append(out, val)
		}
	}
	sort.Strings(out)
	return out, nil
}

func BuildMatrix(platforms, archs []string) ([]Target, error) {
	plats, err := ParsePlatforms(platforms)
	if err != nil {
		return nil, err
	}
	arches, err := ParseArchs(archs)
	if err != nil {
		return nil, err
	}

	var targets []Target
	for _, plat := range plats {
		for _, arch := range arches {
			label := plat
			if plat == "darwin" {
				label = "macos"
			}
			targets = append(targets, Target{GOOS: plat, GOARCH: arch, Label: label})
		}
	}
	return targets, nil
}
