package autodetect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

// setupScenarioDir creates a clean temporary directory with the given files and chdirs into it.
// Returns a cleanup function that restores the original working directory and removes the temp dir.
func setupScenarioDir(t *testing.T, files map[string]string) string {
	t.Helper()
	origDir, err := os.Getwd()
	test.Ok(t, err)

	tmpDir, err := os.MkdirTemp("", "drone-cache-python-scenario-*")
	test.Ok(t, err)

	for relPath, content := range files {
		fullPath := filepath.Join(tmpDir, relPath)
		test.Ok(t, os.MkdirAll(filepath.Dir(fullPath), 0755))
		test.Ok(t, os.WriteFile(fullPath, []byte(content), 0644))
	}

	test.Ok(t, os.Chdir(tmpDir))
	t.Cleanup(func() {
		_ = os.Chdir(origDir)
		_ = os.RemoveAll(tmpDir)
	})

	return tmpDir
}

// --- SINGLE-TOOL SCENARIOS ---

// Scenario 1: Poetry project (e.g. pendulum, poetry, cleo)
func TestScenario_SingleTool_Poetry(t *testing.T) {
	isolatePythonEnv(t)
	setupScenarioDir(t, map[string]string{
		"pyproject.toml": "[tool.poetry]\nname = \"demo-poetry\"\nversion = \"0.1.0\"\n",
		"poetry.lock":    "[[package]]\nname = \"pendulum\"\nversion = \"2.1.2\"\n",
		"src/main.py":    "print('hello from poetry')\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"python-poetry"}, tools)
	test.Assert(t, hash != "", "expected non-empty cache hash")
	test.Equals(t, 1, len(dirs))
	assertCachesManagers(t, dirs, "poetry")

	// Verify poetry.toml was generated with cache-dir
	poetryToml, err := os.ReadFile("poetry.toml")
	test.Ok(t, err)
	test.Assert(t, strings.Contains(string(poetryToml), "cache-dir = \""), "expected cache-dir in poetry.toml")
}

// Scenario 2: uv project (e.g. ruff, modern fast Python projects)
func TestScenario_SingleTool_Uv(t *testing.T) {
	isolatePythonEnv(t)
	setupScenarioDir(t, map[string]string{
		"pyproject.toml": "[project]\nname = \"demo-uv\"\nversion = \"0.1.0\"\n",
		"uv.lock":        "version = 1\nrevision = 1\n",
		"src/main.py":    "print('hello from uv')\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"python-uv"}, tools)
	test.Assert(t, hash != "", "expected non-empty cache hash")
	test.Equals(t, 1, len(dirs))
	assertCachesManagers(t, dirs, "uv")

	// uv should configure [tool.uv] in pyproject.toml when uv.toml is absent
	content, err := os.ReadFile("pyproject.toml")
	test.Ok(t, err)
	test.Assert(t, strings.Contains(string(content), "[tool.uv]"), "expected [tool.uv] in pyproject.toml")
	test.Assert(t, strings.Contains(string(content), "cache-dir = \""), "expected cache-dir in pyproject.toml")
}

// Scenario 3: Pipenv project (e.g. legacy requests, pipenv-based applications)
func TestScenario_SingleTool_Pipenv(t *testing.T) {
	isolatePythonEnv(t)
	setupScenarioDir(t, map[string]string{
		"Pipfile":      "[[source]]\nurl = \"https://pypi.org/simple\"\n",
		"Pipfile.lock": "{\"_meta\": {\"hash\": {\"sha256\": \"12345\"}}}\n",
		"app.py":       "print('hello from pipenv')\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"python-pipenv"}, tools)
	test.Assert(t, hash != "", "expected non-empty cache hash")
	test.Equals(t, 1, len(dirs))
	assertCachesManagers(t, dirs, "pipenv")

	// Pipenv should configure PIPENV_CACHE_DIR in .env
	dotenv, err := os.ReadFile(".env")
	test.Ok(t, err)
	test.Assert(t, strings.Contains(string(dotenv), "PIPENV_CACHE_DIR="), "expected PIPENV_CACHE_DIR in .env")
}

// Scenario 4: pip standard requirements.txt (e.g. Django/Flask standard projects)
func TestScenario_SingleTool_Pip_Requirements(t *testing.T) {
	isolatePythonEnv(t)
	setupScenarioDir(t, map[string]string{
		"requirements.txt": "flask>=2.0.0\nrequests>=2.25.0\n",
		"main.py":          "print('hello from pip')\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"python-pip"}, tools)
	test.Assert(t, hash != "", "expected non-empty cache hash")
	test.Equals(t, 1, len(dirs))
	assertCachesManagers(t, dirs, "pip")

	// pip should configure [global] cache-dir in pip.conf
	pipConf, err := os.ReadFile("pip.conf")
	test.Ok(t, err)
	test.Assert(t, strings.Contains(string(pipConf), "cache-dir = "), "expected cache-dir in pip.conf")
}

// Scenario 5: pip constraints.txt (e.g. Airflow workflow constraints)
func TestScenario_SingleTool_Pip_Constraints(t *testing.T) {
	isolatePythonEnv(t)
	setupScenarioDir(t, map[string]string{
		"constraints.txt": "numpy==1.21.0\npandas==1.3.0\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"python-pip"}, tools)
	test.Assert(t, hash != "", "expected non-empty cache hash")
	test.Equals(t, 1, len(dirs))
	assertCachesManagers(t, dirs, "pip")
}

// Scenario 6: PEP 517/621 pyproject.toml alone (e.g. setuptools/flit pure library without lockfile)
func TestScenario_SingleTool_Pip_PyprojectAlone(t *testing.T) {
	isolatePythonEnv(t)
	setupScenarioDir(t, map[string]string{
		"pyproject.toml": "[build-system]\nrequires = [\"setuptools>=61.0\"]\nbuild-backend = \"setuptools.build_meta\"\n[project]\nname = \"mylib\"\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"python-pip"}, tools)
	test.Assert(t, hash != "", "expected non-empty cache hash")
	test.Equals(t, 1, len(dirs))
	assertCachesManagers(t, dirs, "pip")
}

// --- MULTIPLE-TOOLS SCENARIOS ---

// Scenario 7: uv + pip (e.g. uv app with additional requirements.txt for CI/tools)
func TestScenario_MultiTool_Uv_And_Pip(t *testing.T) {
	isolatePythonEnv(t)
	setupScenarioDir(t, map[string]string{
		"uv.lock":          "version = 1\n",
		"pyproject.toml":   "[project]\nname = \"app\"\n",
		"requirements.txt": "flake8==3.9.2\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"python-uv", "python-pip"}, tools)
	test.Assert(t, hash != "", "expected non-empty cache hash")
	test.Equals(t, 2, len(dirs))
	assertCachesManagers(t, dirs, "uv", "pip")

	// Both config files prepared
	test.Assert(t, fileExists("pip.conf"), "expected pip.conf")
	pyprojectContent, err := os.ReadFile("pyproject.toml")
	test.Ok(t, err)
	test.Assert(t, strings.Contains(string(pyprojectContent), "tool.uv"), "expected tool.uv in pyproject.toml")
}

// Scenario 8: Poetry + pip (e.g. Poetry app with docs/requirements.txt or top-level requirements.txt)
func TestScenario_MultiTool_Poetry_And_Pip(t *testing.T) {
	isolatePythonEnv(t)
	setupScenarioDir(t, map[string]string{
		"pyproject.toml":   "[tool.poetry]\nname = \"app\"\n",
		"poetry.lock":      "[[package]]\nname = \"demo\"\n",
		"requirements.txt": "sphinx>=4.0.0\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"python-poetry", "python-pip"}, tools)
	test.Assert(t, hash != "", "expected non-empty cache hash")
	test.Equals(t, 2, len(dirs))
	assertCachesManagers(t, dirs, "poetry", "pip")
}

// Scenario 9: Pipenv + pip (e.g. Pipfile.lock + requirements.txt)
func TestScenario_MultiTool_Pipenv_And_Pip(t *testing.T) {
	isolatePythonEnv(t)
	setupScenarioDir(t, map[string]string{
		"Pipfile.lock":     "{\"_meta\": {\"hash\": {\"sha256\": \"abc\"}}}\n",
		"requirements.txt": "pytest>=7.0.0\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"python-pipenv", "python-pip"}, tools)
	test.Assert(t, hash != "", "expected non-empty cache hash")
	test.Equals(t, 2, len(dirs))
	assertCachesManagers(t, dirs, "pipenv", "pip")
}

// Scenario 10: uv + Pipenv (e.g. migrating project with both lockfiles)
func TestScenario_MultiTool_Uv_And_Pipenv(t *testing.T) {
	isolatePythonEnv(t)
	setupScenarioDir(t, map[string]string{
		"uv.lock":      "version = 1\n",
		"Pipfile.lock": "{\"_meta\": {\"hash\": {\"sha256\": \"xyz\"}}}\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"python-uv", "python-pipenv"}, tools)
	test.Assert(t, hash != "", "expected non-empty cache hash")
	test.Equals(t, 2, len(dirs))
	assertCachesManagers(t, dirs, "uv", "pipenv")
}

// Scenario 11: Poetry + Pipenv
func TestScenario_MultiTool_Poetry_And_Pipenv(t *testing.T) {
	isolatePythonEnv(t)
	setupScenarioDir(t, map[string]string{
		"poetry.lock":  "[[package]]\nname = \"demo\"\n",
		"Pipfile.lock": "{\"_meta\": {\"hash\": {\"sha256\": \"123\"}}}\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"python-poetry", "python-pipenv"}, tools)
	test.Assert(t, hash != "", "expected non-empty cache hash")
	test.Equals(t, 2, len(dirs))
	assertCachesManagers(t, dirs, "poetry", "pipenv")
}

// Scenario 12: pip deduplication - requirements.txt + constraints.txt + pyproject.toml all present
// Pip should only be detected and prepared ONCE without duplicate cache mounts or configs.
func TestScenario_MultiTool_Pip_Deduplication(t *testing.T) {
	isolatePythonEnv(t)
	setupScenarioDir(t, map[string]string{
		"requirements.txt": "requests>=2.0.0\n",
		"constraints.txt":  "requests==2.28.0\n",
		"pyproject.toml":   "[build-system]\nrequires = [\"setuptools\"]\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	// Only one python-pip tool detected!
	test.Equals(t, []string{"python-pip"}, tools)
	test.Assert(t, hash != "", "expected non-empty cache hash")
	// Exactly one cache directory for pip
	test.Equals(t, 1, len(dirs))
	assertCachesManagers(t, dirs, "pip")
}

// Scenario 13: pyproject.toml is superseded when a lockfile (e.g. poetry.lock) is present.
// pyproject.toml must NOT trigger a second python-pip detection.
func TestScenario_Pyproject_Superseded_By_Lockfile(t *testing.T) {
	isolatePythonEnv(t)
	setupScenarioDir(t, map[string]string{
		"pyproject.toml": "[tool.poetry]\nname = \"my-app\"\n[tool.poetry.dependencies]\npython = \"^3.9\"\n",
		"poetry.lock":    "[[package]]\nname = \"urllib3\"\nversion = \"1.26.5\"\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"python-poetry"}, tools)
	test.Assert(t, hash != "", "expected non-empty cache hash")
	test.Equals(t, 1, len(dirs))
	assertCachesManagers(t, dirs, "poetry")
	test.Assert(t, !fileExists("pip.conf"), "pip.conf should NOT be created when poetry.lock supersedes pyproject.toml")
}
