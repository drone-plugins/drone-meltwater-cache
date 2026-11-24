package autodetect

import (
	"crypto/md5" // #nosec
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func vcpkgMD5(s string) string {
	sum := md5.Sum([]byte(s)) // #nosec
	return hex.EncodeToString(sum[:])
}

func TestScenario_Vcpkg_Detection(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"manifestRoot", map[string]string{"vcpkg.json": `{"dependencies":["fmt"]}`}, true},
		{"manifestNested", map[string]string{"app/vcpkg.json": `{"dependencies":["zlib"]}`}, true},
		{"configOnly", map[string]string{"vcpkg-configuration.json": `{"default-registry":{"kind":"builtin"}}`}, true},
		{"manifestAndConfig", map[string]string{
			"vcpkg.json":               `{"dependencies":["fmt"]}`,
			"vcpkg-configuration.json": `{"default-registry":{"kind":"builtin"}}`,
		}, true},
		{"tooDeep", map[string]string{"a/b/vcpkg.json": `{}`}, false},
		{"plainCmake", map[string]string{"CMakeLists.txt": "project(app)"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := vcpkgWorkspace(t)
			for rel, content := range tc.files {
				vcpkgWrite(t, filepath.Join(root, rel), content)
			}
			dirs, tools, _, err := DetectDirectoriesToCache(false)
			test.Ok(t, err)
			if tc.want {
				test.Equals(t, []string{"vcpkg"}, tools)
				test.Equals(t, []string{filepath.Join(root, ".cache", "vcpkg")}, dirs)
			} else {
				test.Equals(t, 0, len(tools))
			}
		})
	}
}

func TestScenario_Vcpkg_Keys(t *testing.T) {
	root := vcpkgWorkspace(t)
	manifest := `{"name":"demo","dependencies":["fmt"]}`
	config := `{"default-registry":{"kind":"builtin","baseline":"abc123"}}`
	vcpkgWrite(t, filepath.Join(root, "vcpkg.json"), manifest)
	vcpkgWrite(t, filepath.Join(root, "vcpkg-configuration.json"), config)

	_, tools, key1, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{"vcpkg"}, tools)
	// additionalHashGlob (vcpkg-configuration.json) is appended before globToDetect (vcpkg.json)
	test.Equals(t, vcpkgMD5(config)+vcpkgMD5(manifest), key1)

	// Editing either vcpkg.json or vcpkg-configuration.json invalidates the key.
	vcpkgWrite(t, filepath.Join(root, "vcpkg.json"), `{"name":"demo","dependencies":["fmt","zlib"]}`)
	_, _, key2, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Assert(t, key1 != key2, "changing vcpkg.json must change key")

	vcpkgWrite(t, filepath.Join(root, "vcpkg-configuration.json"), `{"default-registry":{"kind":"builtin","baseline":"def456"}}`)
	_, _, key3, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Assert(t, key2 != key3, "changing vcpkg-configuration.json must change key")
}
