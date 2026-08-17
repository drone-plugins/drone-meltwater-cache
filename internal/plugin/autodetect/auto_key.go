package autodetect

import (
	"os"
	"path/filepath"
	"strings"
)

const autoKeyFile = "cache-intelligence-auto-key"

func findGitDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := wd
	for {
		gitPath := filepath.Join(dir, ".git")
		info, err := os.Stat(gitPath)
		if err == nil {
			if info.IsDir() {
				return gitPath
			}
			if parsed := parseGitDirFile(gitPath); parsed != "" {
				return parsed
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func parseGitDirFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 7 || !strings.EqualFold(line[:7], "gitdir:") {
			continue
		}
		p := strings.TrimSpace(line[7:])
		if p == "" {
			return ""
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(filepath.Dir(path), p)
		}
		info, err := os.Stat(p)
		if err == nil && info.IsDir() {
			return p
		}
		return ""
	}
	return ""
}

func autoKeyPath() (string, bool) {
	dir := findGitDir()
	if dir == "" {
		return "", false
	}
	return filepath.Join(dir, autoKeyFile), true
}

// WriteAutoKeySidecar stores the Restore autodetection hash under .git
// so Save can reuse it on the shared workspace without dirtying git status.
func WriteAutoKeySidecar(hashes string) error {
	if hashes == "" {
		return nil
	}
	path, ok := autoKeyPath()
	if !ok {
		return nil
	}
	return os.WriteFile(path, []byte(hashes), 0o644)
}

// ReadAutoKeySidecar returns the Restore hash if the sidecar exists.
func ReadAutoKeySidecar() (string, bool, error) {
	path, ok := autoKeyPath()
	if !ok {
		return "", false, nil
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "", false, nil
	}
	return s, true, nil
}
