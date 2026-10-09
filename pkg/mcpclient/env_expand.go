package mcpclient

import (
	"os"
	"regexp"
	"strings"
)

var envPattern = regexp.MustCompile(`\$\{([^}:]+)(?::-([^}]*))?\}`)

// expandEnv replaces ${VAR} and ${VAR:-default} with environment values.
func expandEnv(s string) string {
	if s == "" || !strings.Contains(s, "${") {
		return s
	}
	return envPattern.ReplaceAllStringFunc(s, func(match string) string {
		sub := envPattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		name := sub[1]
		def := ""
		if len(sub) > 2 {
			def = sub[2]
		}
		if v, ok := os.LookupEnv(name); ok && v != "" {
			return v
		}
		return def
	})
}
