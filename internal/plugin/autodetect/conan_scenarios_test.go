package autodetect

import (
	"crypto/md5" // #nosec
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func conanMD5(s string) string {
	sum := md5.Sum([]byte(s)) // #nosec
	return hex.EncodeToString(sum[:])
}

func TestScenario_Conan_Detection(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"conanLock", map[string]string{"conan.lock": `{"version":"0.5"}`}, true},
		{"conanfileTxt", map[string]string{"conanfile.txt": "[requires]\nfmt/11.0.2\n"}, true},
		{"conanfilePy", map[string]string{"conanfile.py": "from conan import ConanFile\n"}, true},
		{"nested", map[string]string{"cpp/conanfile.txt": "[requires]\nzlib/1.3.1\n"}, true},
		{"tooDeep", map[string]string{"a/b/conanfile.txt": "[requires]\n"}, false},
		{"plainReadme", map[string]string{"README.md": "hello"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := conanWorkspace(t)
			for rel, content := range tc.files {
				conanWrite(t, filepath.Join(root, rel), content)
			}
			dirs, tools, _, err := DetectDirectoriesToCache(false)
			test.Ok(t, err)
			if tc.want {
				test.Equals(t, []string{"conan"}, tools)
				// Only <CONAN_HOME>/p is cached — never credentials.json, global.conf, or profiles.
				test.Equals(t, []string{filepath.Join(root, ".conan2", "p")}, dirs)
			} else {
				test.Equals(t, 0, len(tools))
			}
		})
	}
}

func TestScenario_Conan_PrecedenceAndKeys(t *testing.T) {
	root := conanWorkspace(t)
	lock := `{"version":"0.5","requires":["fmt/11.0.2#abc"]}`
	txt := "[requires]\nfmt/11.0.2\n"
	py := "from conan import ConanFile\n"

	conanWrite(t, filepath.Join(root, "conan.lock"), lock)
	conanWrite(t, filepath.Join(root, "conanfile.txt"), txt)
	conanWrite(t, filepath.Join(root, "conanfile.py"), py)

	_, tools, keyLock, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{"conan"}, tools)
	test.Equals(t, conanMD5(lock), keyLock)

	// Without conan.lock, conanfile.txt wins over conanfile.py.
	root2 := conanWorkspace(t)
	conanWrite(t, filepath.Join(root2, "conanfile.txt"), txt)
	conanWrite(t, filepath.Join(root2, "conanfile.py"), py)
	_, tools2, keyTxt, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{"conan"}, tools2)
	test.Equals(t, conanMD5(txt), keyTxt)
}
