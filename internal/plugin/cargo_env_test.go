package plugin

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-kit/log"
	"github.com/meltwater/drone-cache/test"
)

func TestExecCargoWarnsWhenCargoHomeUnset(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)
	t.Setenv("CARGO_HOME", "")

	test.Ok(t, os.WriteFile("Cargo.lock", []byte("# dummy lock"), 0644))

	cacheRoot := t.TempDir()

	// Test Restore step with CARGO_HOME unset -> expect warning
	buf := &bytes.Buffer{}
	pRestore := autoDetectPlugin(t, cacheRoot, true, false)
	pRestore.logger = log.NewLogfmtLogger(buf)
	_ = pRestore.Exec()

	logOutput := buf.String()
	test.Assert(t, strings.Contains(logOutput, "level=warn"), "expected warn log level, got: %s", logOutput)
	test.Assert(t, strings.Contains(logOutput, "CARGO_HOME environment variable is not set"), "expected CARGO_HOME warning, got: %s", logOutput)

	// Test Rebuild/Save step with CARGO_HOME unset -> expect warning
	buf.Reset()
	pRebuild := autoDetectPlugin(t, cacheRoot, false, true)
	pRebuild.logger = log.NewLogfmtLogger(buf)
	test.Ok(t, os.MkdirAll(filepath.Join(workspace, ".cache", "cargo", "registry", "index"), 0755))
	_ = pRebuild.Exec()

	logOutput = buf.String()
	test.Assert(t, strings.Contains(logOutput, "level=warn"), "expected warn log level on rebuild, got: %s", logOutput)
	test.Assert(t, strings.Contains(logOutput, "CARGO_HOME environment variable is not set"), "expected CARGO_HOME warning on rebuild, got: %s", logOutput)
}

func TestExecCargoNoWarnWhenCargoHomeSet(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)
	t.Setenv("CARGO_HOME", filepath.Join(workspace, ".cache", "cargo"))

	test.Ok(t, os.WriteFile("Cargo.lock", []byte("# dummy lock"), 0644))

	cacheRoot := t.TempDir()

	buf := &bytes.Buffer{}
	p := autoDetectPlugin(t, cacheRoot, true, false)
	p.logger = log.NewLogfmtLogger(buf)
	_ = p.Exec()

	logOutput := buf.String()
	test.Assert(t, !strings.Contains(logOutput, "CARGO_HOME environment variable is not set"), "unexpected CARGO_HOME warning when CARGO_HOME is set: %s", logOutput)
}
