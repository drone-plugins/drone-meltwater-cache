package autodetect

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type pythonPreparer struct{}

func newPythonPreparer() *pythonPreparer {
	return &pythonPreparer{}
}

// PrepareRepo injects cache configuration for Python package managers
// to use a repo-local cache directory instead of the user's home directory.
// Priority: poetry.lock > Pipfile.lock > requirements.txt
// This enables caching to work in containerized environments where only
// the repository directory is mounted across CI steps.
func (*pythonPreparer) PrepareRepo(dir string) (string, error) {
	// Check poetry.lock (pyproject.toml should exist alongside)
	if fileExists(filepath.Join(dir, "poetry.lock")) {
		return preparePoetry(dir)
	}

	// Check Pipfile.lock
	if fileExists(filepath.Join(dir, "Pipfile.lock")) {
		return preparePipenv(dir)
	}

	// Check requirements.txt or pyproject.toml (pip)
	return preparePip(dir)
}

// preparePip modifies setup.cfg to redirect pip cache to repo-local directory
func preparePip(dir string) (string, error) {
	cacheDir := filepath.Join(dir, ".cache", "pip")
	setupCfg := filepath.Join(dir, "setup.cfg")
	cacheConfig := fmt.Sprintf("[global]\ncache-dir = %s\n", cacheDir)

	if _, err := os.Stat(setupCfg); errors.Is(err, os.ErrNotExist) {
		// Create setup.cfg with [global] section and cache-dir
		f, err := os.Create(setupCfg)
		if err != nil {
			return "", err
		}
		defer f.Close()

		_, err = f.WriteString(cacheConfig)
		if err != nil {
			return "", err
		}

		return cacheDir, nil
	} else if err != nil {
		return "", err
	}

	// setup.cfg exists, check if file ends with newline (similar to gradle.properties handling)
	info, err := os.Stat(setupCfg)
	if err != nil {
		return "", err
	}

	contentToAppend := cacheConfig
	if info.Size() > 0 {
		buf := make([]byte, 1)
		f, err := os.Open(setupCfg)
		if err != nil {
			return "", err
		}
		_, err = f.ReadAt(buf, info.Size()-1)
		f.Close()
		if err != nil {
			return "", err
		}
		if buf[0] != '\n' {
			contentToAppend = "\n" + contentToAppend
		}
	}

	// Append to existing setup.cfg
	f, err := os.OpenFile(setupCfg, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) //nolint:gomnd
	if err != nil {
		return "", err
	}
	defer f.Close()

	_, err = f.WriteString(contentToAppend)
	if err != nil {
		return "", err
	}

	return cacheDir, nil
}

// preparePoetry modifies pyproject.toml to redirect poetry cache to repo-local directory
func preparePoetry(dir string) (string, error) {
	cacheDir := filepath.Join(dir, ".cache", "poetry")
	pyprojectPath := filepath.Join(dir, "pyproject.toml")

	// Read existing pyproject.toml
	content, err := os.ReadFile(pyprojectPath)
	if err != nil {
		return "", err
	}

	contentStr := string(content)
	cacheConfig := fmt.Sprintf("cache-dir = \"%s\"", cacheDir)

	// Check if [tool.poetry] section exists
	toolPoetryExists := strings.Contains(contentStr, "[tool.poetry]")

	if !toolPoetryExists {
		// Add [tool.poetry] section with cache-dir
		contentStr += fmt.Sprintf("\n[tool.poetry]\n%s\n", cacheConfig)
	} else if !strings.Contains(contentStr, "cache-dir") {
		// [tool.poetry] exists but cache-dir is not set, add it after the section header
		contentStr = strings.Replace(
			contentStr,
			"[tool.poetry]",
			fmt.Sprintf("[tool.poetry]\n%s", cacheConfig),
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

// preparePipenv modifies Pipfile to redirect pipenv cache to repo-local directory
// If Pipfile doesn't exist, creates a setup script to set PIPENV_CACHE_DIR
func preparePipenv(dir string) (string, error) {
	cacheDir := filepath.Join(dir, ".cache", "pipenv")
	pipfilePath := filepath.Join(dir, "Pipfile")

	// Try to read existing Pipfile
	if fileExists(pipfilePath) {
		content, err := os.ReadFile(pipfilePath)
		if err != nil {
			return "", err
		}

		contentStr := string(content)

		// Check if [requires] section exists, if not create it
		if !strings.Contains(contentStr, "[requires]") {
			contentStr += fmt.Sprintf("\n[requires]\npip_version = \">=20.0\"\n")
		}

		// Add cache-dir comment as documentation since Pipfile doesn't support it directly
		// Instead, create a .env.local file for cache setup
		if !strings.Contains(contentStr, "cache-dir") {
			contentStr += fmt.Sprintf("\n# Cache configuration: export PIPENV_CACHE_DIR=%s\n", cacheDir)
		}

		f, err := os.Create(pipfilePath)
		if err != nil {
			return "", err
		}
		defer f.Close()

		_, err = f.WriteString(contentStr)
		if err != nil {
			return "", err
		}
	}

	// Create .env.local to set PIPENV_CACHE_DIR for any pipenv commands
	envLocalPath := filepath.Join(dir, ".env.local")
	envContent := "export PIPENV_CACHE_DIR=" + cacheDir + "\n"

	f, err := os.Create(envLocalPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	_, err = f.WriteString(envContent)
	if err != nil {
		return "", err
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(cacheDir, os.ModePerm); err != nil {
		return "", err
	}

	return cacheDir, nil
}

// fileExists checks if a file exists at the given path
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
