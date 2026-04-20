package autodetect

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func TestDotnetPreparerRespectsNugetPackagesEnvVar(t *testing.T) {
	origEnv := os.Getenv("NUGET_PACKAGES")
	os.Setenv("NUGET_PACKAGES", "/custom/cache/path")
	defer os.Setenv("NUGET_PACKAGES", origEnv)

	dir, err := os.MkdirTemp("", "dotnet-env-*")
	test.Ok(t, err)
	defer os.RemoveAll(dir)

	preparer := newDotnetPreparer()
	pathToCache, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)

	test.Equals(t, "/custom/cache/path", pathToCache)

	// nuget.config should NOT have been created
	_, err = os.Stat(filepath.Join(dir, "nuget.config"))
	test.Assert(t, os.IsNotExist(err), "nuget.config should not be created when NUGET_PACKAGES is set")
}

func TestDotnetPreparerNugetPackagesRelativePath(t *testing.T) {
	origEnv := os.Getenv("NUGET_PACKAGES")
	os.Setenv("NUGET_PACKAGES", "relative/cache/path")
	defer os.Setenv("NUGET_PACKAGES", origEnv)

	dir, err := os.MkdirTemp("", "dotnet-rel-*")
	test.Ok(t, err)
	defer os.RemoveAll(dir)

	preparer := newDotnetPreparer()
	pathToCache, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)

	// Should resolve to absolute path
	test.Assert(t, filepath.IsAbs(pathToCache),
		"returned path should be absolute, got: %s", pathToCache)

	expectedAbs, _ := filepath.Abs("relative/cache/path")
	test.Equals(t, expectedAbs, pathToCache)
}

func TestDotnetPreparerFallsBackToNugetConfig(t *testing.T) {
	origEnv := os.Getenv("NUGET_PACKAGES")
	os.Unsetenv("NUGET_PACKAGES")
	defer os.Setenv("NUGET_PACKAGES", origEnv)

	dir, err := os.MkdirTemp("", "dotnet-fallback-*")
	test.Ok(t, err)
	defer os.RemoveAll(dir)

	preparer := newDotnetPreparer()
	pathToCache, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)

	expectedPath := filepath.Join(dir, ".nuget", "packages")
	test.Equals(t, expectedPath, pathToCache)

	// nuget.config should have been created
	configPath := filepath.Join(dir, "nuget.config")
	_, err = os.Stat(configPath)
	test.Assert(t, !os.IsNotExist(err), "nuget.config should be created when NUGET_PACKAGES is not set")

	// Verify the config contains globalPackagesFolder
	data, err := os.ReadFile(configPath)
	test.Ok(t, err)

	var config Configuration
	test.Ok(t, xml.Unmarshal(data, &config))
	test.Assert(t, config.Config != nil, "config section should exist")

	found := false
	for _, add := range config.Config.Add {
		if add.Key == "globalPackagesFolder" {
			test.Equals(t, expectedPath, add.Value)
			found = true
		}
	}
	test.Assert(t, found, "globalPackagesFolder should be set in nuget.config")
}

func TestDotnetPreparerExistingNugetConfig(t *testing.T) {
	origEnv := os.Getenv("NUGET_PACKAGES")
	os.Unsetenv("NUGET_PACKAGES")
	defer os.Setenv("NUGET_PACKAGES", origEnv)

	dir, err := os.MkdirTemp("", "dotnet-existing-*")
	test.Ok(t, err)
	defer os.RemoveAll(dir)

	expectedPath := filepath.Join(dir, ".nuget", "packages")

	// Create existing nuget.config with the correct globalPackagesFolder
	existingConfig := Configuration{
		Config: &Config{
			Add: []Add{
				{Key: "globalPackagesFolder", Value: expectedPath},
			},
		},
	}
	configData, err := xml.MarshalIndent(existingConfig, "", "  ")
	test.Ok(t, err)
	test.Ok(t, os.WriteFile(filepath.Join(dir, "nuget.config"), configData, 0644))

	preparer := newDotnetPreparer()
	pathToCache, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)

	test.Equals(t, expectedPath, pathToCache)

	// Verify no duplicate entries were added
	data, err := os.ReadFile(filepath.Join(dir, "nuget.config"))
	test.Ok(t, err)

	var config Configuration
	test.Ok(t, xml.Unmarshal(data, &config))

	count := 0
	for _, add := range config.Config.Add {
		if add.Key == "globalPackagesFolder" {
			count++
		}
	}
	test.Equals(t, 1, count)
}

func TestDotnetPreparerEnvVarTakesPrecedenceOverConfig(t *testing.T) {
	origEnv := os.Getenv("NUGET_PACKAGES")
	os.Setenv("NUGET_PACKAGES", "/env/path")
	defer os.Setenv("NUGET_PACKAGES", origEnv)

	dir, err := os.MkdirTemp("", "dotnet-precedence-*")
	test.Ok(t, err)
	defer os.RemoveAll(dir)

	// Create existing nuget.config with a different path
	existingConfig := Configuration{
		Config: &Config{
			Add: []Add{
				{Key: "globalPackagesFolder", Value: "/some/other/path"},
			},
		},
	}
	configData, err := xml.MarshalIndent(existingConfig, "", "  ")
	test.Ok(t, err)
	configPath := filepath.Join(dir, "nuget.config")
	test.Ok(t, os.WriteFile(configPath, configData, 0644))

	// Save original config content for comparison
	originalContent, err := os.ReadFile(configPath)
	test.Ok(t, err)

	preparer := newDotnetPreparer()
	pathToCache, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)

	// Env var should win
	test.Equals(t, "/env/path", pathToCache)

	// nuget.config should not have been modified
	afterContent, err := os.ReadFile(configPath)
	test.Ok(t, err)
	test.Equals(t, string(originalContent), string(afterContent))
}

func TestDotnetPreparerEmptyEnvVarFallsBack(t *testing.T) {
	origEnv := os.Getenv("NUGET_PACKAGES")
	os.Setenv("NUGET_PACKAGES", "")
	defer os.Setenv("NUGET_PACKAGES", origEnv)

	dir, err := os.MkdirTemp("", "dotnet-empty-*")
	test.Ok(t, err)
	defer os.RemoveAll(dir)

	preparer := newDotnetPreparer()
	pathToCache, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)

	// Should fall back to nuget.config behavior
	expectedPath := filepath.Join(dir, ".nuget", "packages")
	test.Equals(t, expectedPath, pathToCache)
}
