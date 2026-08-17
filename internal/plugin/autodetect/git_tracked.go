package autodetect

import (
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

func gitTrackedNPMFiles() (map[string]struct{}, bool) {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		return nil, false
	}

	args := []string{
		"ls-files", "-z", "--cached", "--",
		packageJSONFile,
		packageLockFile,
		npmShrinkwrapFile,
		"*/" + packageJSONFile,
		"*/" + packageLockFile,
		"*/" + npmShrinkwrapFile,
	}
	cmd = exec.Command("git", args...)
	cmd.Stderr = io.Discard
	out, err = cmd.Output()
	if err != nil {
		return nil, false
	}

	tracked := make(map[string]struct{})
	for _, p := range strings.Split(string(out), "\x00") {
		if p == "" {
			continue
		}
		tracked[filepath.Clean(p)] = struct{}{}
	}
	return tracked, true
}

func npmFingerprintExists(path string, tracked map[string]struct{}, useGit bool) bool {
	if !fileExists(path) {
		return false
	}
	if !useGit {
		return true
	}
	_, ok := tracked[filepath.Clean(path)]
	return ok
}
