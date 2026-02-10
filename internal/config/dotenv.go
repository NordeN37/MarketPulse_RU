package config

import (
	"bufio"
	"os"
	"strings"
)

// loadEnvFile reads a .env file and sets variables into the process environment.
// Supports formats: VAR=value, export VAR=value, VAR="value", VAR='value'.
// Lines starting with # are comments. Empty lines are skipped.
func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Strip "export " prefix.
		line = strings.TrimPrefix(line, "export ")
		line = strings.TrimSpace(line)

		// Split on first '='.
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])

		// Strip surrounding quotes.
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}

		// Only set if not already defined in environment (env takes precedence).
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
	return scanner.Err()
}
