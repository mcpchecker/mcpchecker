package mcpclient

import (
	"os"
	"regexp"
	"strings"
)

// expandEnv replaces ${VAR} and ${VAR:-default} with environment values.
func expandEnv(s string) string {
	if s == "" || !strings.Contains(s, "${") {
		return s
	}
	re := regexp.MustCompile(`\$\{([^}:]+)(?::-([^}]*))?\}`)
	return re.ReplaceAllStringFunc(s, func(match string) string {
		sub := re.FindStringSubmatch(match)
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
