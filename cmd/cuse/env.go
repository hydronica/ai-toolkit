package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// loadEnvFile reads KEY=VALUE pairs from path and sets them in the process
// environment. Missing file is silently ignored. Quotes and whitespace are
// trimmed. Already-set variables are overwritten.
func loadEnvFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading env file %s: %w", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.Index(line, "=")
		if i <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		val = strings.Trim(val, `"'`)
		if key != "" {
			_ = os.Setenv(key, val)
		}
	}
	return nil
}

// writeEnvKey upserts key=value in the .env file at path.
// If the key already exists on a line it is replaced; otherwise it is appended.
// The file is created if it does not exist.
func writeEnvKey(path, key, value string) error {
	var lines []string
	found := false

	f, err := os.Open(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("opening env file: %w", err)
	}
	if err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, key+"=") || strings.HasPrefix(line, key+`="`) {
				line = key + `="` + value + `"`
				found = true
			}
			lines = append(lines, line)
		}
		_ = f.Close()
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("reading env file: %w", err)
		}
	}

	if !found {
		lines = append(lines, key+`="`+value+`"`)
	}

	out := strings.Join(lines, "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	if err := os.WriteFile(path, []byte(out), 0o600); err != nil {
		return fmt.Errorf("writing env file: %w", err)
	}
	return nil
}
