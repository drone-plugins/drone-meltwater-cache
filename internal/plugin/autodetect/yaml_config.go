package autodetect

import (
	"bufio"
	"errors"
	"os"
	"strings"
)

// yamlTopLevelString returns a scalar top-level key from a YAML file. It only
// understands the flat `key: value` settings that pnpm and Yarn configs use.
func yamlTopLevelString(fileName, key string) (string, error) {
	f, err := os.Open(fileName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}

		return "", err
	}
	defer f.Close()

	var value string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), " \t\r")
		if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
			continue
		}

		name, rest, found := strings.Cut(line, ":")
		if !found || strings.Trim(strings.TrimSpace(name), `"'`) != key {
			continue
		}

		value = yamlScalar(rest)
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return value, nil
}

func yamlScalar(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if quote := raw[0]; quote == '"' || quote == '\'' {
		if end := strings.IndexByte(raw[1:], quote); end >= 0 {
			return raw[1 : end+1]
		}
	}

	if i := strings.Index(raw, " #"); i >= 0 {
		raw = raw[:i]
	}

	return strings.TrimSpace(raw)
}
