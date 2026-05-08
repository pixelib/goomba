package util

import (
	"strings"
)

func EnvWithOverrides(base []string, overrides map[string]string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		parts := strings.SplitN(entry, "=", 2)
		key := parts[0]
		if val, ok := overrides[key]; ok {
			out = append(out, key+"="+val)
			seen[key] = true
			continue
		}
		out = append(out, entry)
		seen[key] = true
	}
	for key, val := range overrides {
		if !seen[key] {
			out = append(out, key+"="+val)
		}
	}

	return out
}
