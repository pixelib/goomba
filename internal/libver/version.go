package libver

import (
	_ "embed"
	"strings"
)

//go:embed version.txt
var versionData string

func HasVersion() bool {
	return strings.TrimSpace(versionData) != ""
}

// GetVersion returns the version string, or "" if not set.
func GetVersion() string {
	v := strings.TrimSpace(versionData)
	if v == "" {
		return ""
	}
	return v
}
