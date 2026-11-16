package autodetect

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func sbtWrite(t *testing.T, path, content string) {
	t.Helper()
	test.Ok(t, os.MkdirAll(filepath.Dir(path), 0755))
	test.Ok(t, os.WriteFile(path, []byte(content), 0644))
}

func TestScenario_Sbt_Detection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		files    []string
		detected bool
	}{
		{"root", []string{"build.sbt.lock"}, true},
		{"nested", []string{"app/build.sbt.lock"}, true},
		{"deep", []string{"a/b/build.sbt.lock"}, false},
		{"manifest", []string{"build.sbt"}, false},
		{"customLock", []string{"build.scala-2.13.sbt.lock"}, false},
		{"rootAndNested", []string{"build.sbt.lock", "app/build.sbt.lock"}, true},
		{"siblings", []string{"a/build.sbt.lock", "b/build.sbt.lock"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := sbtWorkspace(t)
			for i, f := range tc.files {
				sbtWrite(t, f, fmt.Sprintf("lock-%d", i))
			}
			dirs, tools, key, err := DetectDirectoriesToCache(false)
			test.Ok(t, err)
			if !tc.detected {
				test.Equals(t, 0, len(tools))
				test.Equals(t, "", key)
				return
			}
			test.Equals(t, []string{"sbt"}, tools)
			expected := "lock-0"
			if tc.name == "siblings" {
				expected = "lock-1"
			} // legacy equal-length tie chooses the later match
			test.Equals(t, fmt.Sprintf("%x", md5.Sum([]byte(expected))), key)
			test.Equals(t, []string{filepath.Join(root, ".cache", "sbt", "https")}, dirs)
		})
	}
}

func TestScenario_Sbt_Keys(t *testing.T) {
	sbtWorkspace(t)
	sbtWrite(t, "build.sbt.lock", "first")
	_, _, first, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	sbtWrite(t, "build.sbt", "scalaVersion := \"3.3.7\"")
	_, _, same, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, first, same)
	sbtWrite(t, "build.sbt.lock", "second")
	_, _, changed, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Assert(t, first != changed, "lock must invalidate")
	test.Equals(t, fmt.Sprintf("%x", md5.Sum([]byte("second"))), changed)
}

func TestScenario_Sbt_Mixed(t *testing.T) {
	sbtWorkspace(t)
	sbtWrite(t, "pom.xml", "<project/>")
	legacyDirs, legacyTools, legacyKey, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{"maven"}, legacyTools)
	test.Equals(t, fmt.Sprintf("%x", md5.Sum([]byte("<project/>"))), legacyKey)
	sbtWrite(t, "build.sbt.lock", "lock")
	dirs, tools, key, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{"maven", "sbt"}, tools)
	test.Equals(t, legacyDirs[0], dirs[0])
	test.Equals(t, legacyKey+fmt.Sprintf("%x", md5.Sum([]byte("lock"))), key)
	test.Equals(t, 2, len(dirs))
}

func TestScenario_Sbt_Skip(t *testing.T) {
	sbtWorkspace(t)
	sbtWrite(t, "build.sbt.lock", "lock")
	dirs, tools, key, err := DetectDirectoriesToCache(true)
	test.Ok(t, err)
	test.Equals(t, 0, len(dirs))
	test.Equals(t, 0, len(tools))
	test.Equals(t, "", key)
}
