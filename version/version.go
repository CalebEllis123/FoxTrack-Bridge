package version

import (
	"strconv"
	"strings"
)

// AppVersion is injected at build time via -ldflags.
var AppVersion = "dev"

func Normalized(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if idx := strings.IndexAny(v, "+-"); idx >= 0 {
		v = v[:idx]
	}
	if v == "" {
		return "0.0.0"
	}
	return v
}

func Compare(a, b string) int {
	ap := parseParts(Normalized(a))
	bp := parseParts(Normalized(b))
	for i := 0; i < 3; i++ {
		if ap[i] < bp[i] {
			return -1
		}
		if ap[i] > bp[i] {
			return 1
		}
	}
	return 0
}

func parseParts(v string) [3]int {
	parts := strings.Split(v, ".")
	var out [3]int
	for i := 0; i < len(out) && i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err == nil && n >= 0 {
			out[i] = n
		}
	}
	return out
}
