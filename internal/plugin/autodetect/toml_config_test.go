package autodetect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

// writeTOML writes an initial TOML file for a test and returns its path.
func writeTOML(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	test.Ok(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func readTOML(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	test.Ok(t, err)
	return string(b)
}

// TestUpsertTOMLCommentedHeader is the regression test for the duplicate-table
// bug: a table header with a trailing comment must still be recognized so the
// key is inserted into the existing section instead of a second one being
// appended (which yields invalid TOML that uv/poetry refuse to parse).
func TestUpsertTOMLCommentedHeader(t *testing.T) {
	path := writeTOML(t, "pyproject.toml",
		"[project]\nname = \"x\"\n\n[tool.uv]  # our uv settings\nindexes = []\n")

	test.Ok(t, upsertTOMLString(path, "tool.uv", "cache-dir", "/repo/.cache/uv"))

	out := readTOML(t, path)
	test.Equals(t, 1, strings.Count(out, "[tool.uv]"))
	test.Assert(t, strings.Contains(out, "# our uv settings"), "expected the header comment to be preserved")
	test.Assert(t, strings.Contains(out, "indexes = []"), "expected existing key to be preserved")
	test.Assert(t, strings.Contains(out, `cache-dir = "/repo/.cache/uv"`), "expected cache-dir to be added")
}

// TestUpsertTOMLQuotedKeyWithHash ensures a '#' inside a bracketed (quoted) key
// is not mistaken for a comment: only text after the closing ']' is a comment.
func TestUpsertTOMLQuotedKeyWithHash(t *testing.T) {
	path := writeTOML(t, "uv.toml", "[\"weird#section\"]\nk = 1\n")

	// Root write must land before the first real table, and the quoted-hash
	// header must remain intact.
	test.Ok(t, upsertTOMLString(path, "", "cache-dir", "/repo/.cache/uv"))

	out := readTOML(t, path)
	test.Assert(t, strings.Contains(out, `["weird#section"]`), "expected quoted header with '#' to be preserved")
	test.Assert(t, strings.Contains(out, `cache-dir = "/repo/.cache/uv"`), "expected cache-dir to be written")
}

// TestUpsertTOMLReplaceInCommentedHeaderSection ensures an existing key inside a
// commented-header section is replaced in place (not duplicated).
func TestUpsertTOMLReplaceInCommentedHeaderSection(t *testing.T) {
	path := writeTOML(t, "pyproject.toml",
		"[tool.uv] # comment\ncache-dir = \"/old\"\n")

	test.Ok(t, upsertTOMLString(path, "tool.uv", "cache-dir", "/new"))

	out := readTOML(t, path)
	test.Equals(t, 1, strings.Count(out, "cache-dir"))
	test.Assert(t, strings.Contains(out, `cache-dir = "/new"`), "expected cache-dir to be replaced")
	test.Assert(t, !strings.Contains(out, "/old"), "expected old value to be gone")
}

// TestUpsertTOMLCreatesFile creates a fresh file with a root-level key.
func TestUpsertTOMLCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "uv.toml")

	test.Ok(t, upsertTOMLString(path, "", "cache-dir", "/c"))

	out := readTOML(t, path)
	test.Equals(t, "cache-dir = \"/c\"\n", out)
}

// TestUpsertTOMLAppendsMissingSection appends a new table when the target
// section does not exist, leaving existing content untouched.
func TestUpsertTOMLAppendsMissingSection(t *testing.T) {
	path := writeTOML(t, "pyproject.toml", "[project]\nname = \"x\"\n")

	test.Ok(t, upsertTOMLString(path, "tool.uv", "cache-dir", "/c"))

	out := readTOML(t, path)
	test.Equals(t, 1, strings.Count(out, "[tool.uv]"))
	test.Assert(t, strings.Contains(out, "[project]"), "expected existing section preserved")
	test.Assert(t, strings.Contains(out, `cache-dir = "/c"`), "expected cache-dir in new section")
}

// TestUpsertTOMLPreservesCRLF keeps Windows line endings intact.
func TestUpsertTOMLPreservesCRLF(t *testing.T) {
	path := writeTOML(t, "uv.toml", "offline = true\r\n")

	test.Ok(t, upsertTOMLString(path, "", "cache-dir", "/c"))

	out := readTOML(t, path)
	test.Assert(t, strings.Contains(out, "\r\n"), "expected CRLF line endings to be preserved")
	test.Assert(t, !strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n"), "expected no bare LF")
	test.Assert(t, strings.Contains(out, `cache-dir = "/c"`), "expected cache-dir to be written")
}

func TestTOMLTableHeader(t *testing.T) {
	cases := []struct {
		line       string
		wantHeader string
		wantOK     bool
	}{
		{"[tool.uv]", "[tool.uv]", true},
		{"[tool.uv]  # note", "[tool.uv]", true},
		{"[tool.uv]#note", "[tool.uv]", true},
		{"[[array.table]]", "[[array.table]]", true},
		{`["weird#key"]`, `["weird#key"]`, true},
		{"cache-dir = \"/x\"", "", false},
		{"# just a comment", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		gotHeader, gotOK := tomlTableHeader(c.line)
		test.Equals(t, c.wantOK, gotOK)
		if c.wantOK {
			test.Equals(t, c.wantHeader, gotHeader)
		}
	}
}
