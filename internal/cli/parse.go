package cli

import (
	"errors"
	"strings"
)

func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func splitArgs(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}

	var out []string
	var buf []rune
	inQuote := rune(0)
	escaped := false
	for _, r := range value {
		switch {
		case escaped:
			buf = append(buf, r)
			escaped = false
		case r == '\\':
			escaped = true
		case inQuote != 0:
			if r == inQuote {
				inQuote = 0
			} else {
				buf = append(buf, r)
			}
		case r == '\'' || r == '"':
			inQuote = r
		case r == ' ' || r == '\t' || r == '\n':
			if len(buf) > 0 {
				out = append(out, string(buf))
				buf = buf[:0]
			}
		default:
			buf = append(buf, r)
		}
	}
	if escaped || inQuote != 0 {
		return nil, errors.New("unterminated quote or escape in --go-args")
	}
	if len(buf) > 0 {
		out = append(out, string(buf))
	}
	return out, nil
}
