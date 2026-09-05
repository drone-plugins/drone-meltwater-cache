package autodetect

import (
	"fmt"
	"os"
	"strings"
)

// upsertTOMLString updates a string key in one exact TOML table. An empty
// section targets the document root. This intentionally handles only the
// simple string settings written by the cache preparers.
func upsertTOMLString(path, section, key, value string) error {
	content, mode, err := readOptionalFile(path)
	if err != nil {
		return err
	}

	newline := "\n"
	if strings.Contains(content, "\r\n") {
		newline = "\r\n"
		content = strings.ReplaceAll(content, "\r\n", "\n")
	}

	var lines []string
	if content != "" {
		lines = strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	}
	setting := fmt.Sprintf("%s = %q", key, value)
	targetHeader := "[" + section + "]"
	inTarget := section == ""
	targetFound := section == ""
	insertAt := len(lines)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if header, ok := tomlTableHeader(trimmed); ok {
			if inTarget {
				insertAt = i
				break
			}
			inTarget = header == targetHeader
			if inTarget {
				targetFound = true
				insertAt = i + 1
			}
			continue
		}

		if inTarget && tomlKeyMatches(trimmed, key) {
			lines[i] = setting
			return writeTOMLLines(path, lines, newline, mode)
		}
	}

	if !targetFound {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, targetHeader, setting)
	} else {
		lines = append(lines[:insertAt], append([]string{setting}, lines[insertAt:]...)...)
	}

	return writeTOMLLines(path, lines, newline, mode)
}

// tomlTableHeader reports whether a trimmed line is a TOML table header and
// returns the header text with any trailing comment removed, so headers like
// "[tool.uv]  # note" are still recognized. Only text after the final ']' is
// treated as a comment, so a '#' inside the brackets (e.g. a quoted key) is
// preserved. Without this, an unrecognized header caused the target section to
// be appended a second time, producing a duplicate table and invalid TOML.
func tomlTableHeader(trimmed string) (string, bool) {
	if lb := strings.LastIndex(trimmed, "]"); lb >= 0 {
		if strings.Contains(trimmed[lb+1:], "#") {
			trimmed = strings.TrimSpace(trimmed[:lb+1])
		}
	}
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		return trimmed, true
	}
	return "", false
}

func tomlKeyMatches(line, key string) bool {
	if strings.HasPrefix(line, "#") {
		return false
	}
	parts := strings.SplitN(line, "=", 2)
	return len(parts) == 2 && strings.TrimSpace(parts[0]) == key
}

func writeTOMLLines(path string, lines []string, newline string, mode os.FileMode) error {
	content := strings.Join(lines, "\n") + "\n"
	if newline == "\r\n" {
		content = strings.ReplaceAll(content, "\n", "\r\n")
	}
	return os.WriteFile(path, []byte(content), mode)
}
