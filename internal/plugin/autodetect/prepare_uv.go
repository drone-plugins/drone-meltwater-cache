package autodetect

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type uvPreparer struct{}

func newUvPreparer() *uvPreparer {
	return &uvPreparer{}
}

// PrepareRepo modifies pyproject.toml to configure uv cache to use repo-local directory.
// This follows the same pattern as Poetry, storing config in pyproject.toml.
func (*uvPreparer) PrepareRepo(dir string) (string, error) {
	cacheDir := filepath.Join(dir, ".cache", "uv")
	pyprojectPath := filepath.Join(dir, "pyproject.toml")

	// Read existing pyproject.toml
	content, err := os.ReadFile(pyprojectPath)
	if err != nil {
		return "", err
	}

	contentStr := string(content)
	cacheConfig := fmt.Sprintf("cache-dir = \"%s\"", cacheDir)

	// Check if [tool.uv] section exists
	toolUvExists := strings.Contains(contentStr, "[tool.uv]")

	if !toolUvExists {
		// Add [tool.uv] section with cache-dir
		contentStr += fmt.Sprintf("\n[tool.uv]\n%s\n", cacheConfig)
	} else if !strings.Contains(contentStr, "cache-dir") {
		// [tool.uv] exists but cache-dir is not set, add it after the section header
		contentStr = strings.Replace(
			contentStr,
			"[tool.uv]",
			fmt.Sprintf("[tool.uv]\n%s", cacheConfig),
			1,
		)
	}

	// Write back to pyproject.toml
	f, err := os.Create(pyprojectPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	_, err = f.WriteString(contentStr)
	if err != nil {
		return "", err
	}

	return cacheDir, nil
}
