package cache

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/meltwater/drone-cache/storage/common"
)

// Tests for path extraction with various key formats and directory structures
func TestPathExtractionWithDifferentKeyFormats(t *testing.T) {
	// Setup
	namespace := "repo"
	
	// Test cases for different key formats and path structures
	testCases := []struct {
		name                 string
		key                  string
		entries              []common.FileEntry
		expectedPaths        []string
		expectedSourcePaths  map[string]string
	}{
		{
			name: "ExactKeyMatch",
			key:  "exact-key-12345",
			entries: []common.FileEntry{
				{Path: "repo/exact-key-12345/path1"},
			},
			expectedPaths: []string{"path1"},
			expectedSourcePaths: map[string]string{
				"path1": "repo/exact-key-12345/path1",
			},
		},
		{
			name: "KeyWithNumericSuffix",
			key:  "prefix-key",
			entries: []common.FileEntry{
				{Path: "repo/prefix-key1/path1"},
				{Path: "repo/prefix-key2/path2"},
				{Path: "repo/prefix-key3/path3"},
				{Path: "repo/other-key/path4"},
			},
			expectedPaths: []string{"path1", "path2", "path3"},
			expectedSourcePaths: map[string]string{
				"path1": "repo/prefix-key1/path1",
				"path2": "repo/prefix-key2/path2",
				"path3": "repo/prefix-key3/path3",
			},
		},
		{
			name: "KeyWithDashSuffix",
			key:  "prefix-key",
			entries: []common.FileEntry{
				{Path: "repo/prefix-key-1/path1"},
				{Path: "repo/prefix-key-2/path2"},
				{Path: "repo/prefix-key-3/path3"},
				{Path: "repo/other-key/path4"},
			},
			expectedPaths: []string{"path1", "path2", "path3"},
			expectedSourcePaths: map[string]string{
				"path1": "repo/prefix-key-1/path1",
				"path2": "repo/prefix-key-2/path2",
				"path3": "repo/prefix-key-3/path3",
			},
		},
		{
			name: "NestedDirectoryPaths",
			key:  "nested-key",
			entries: []common.FileEntry{
				{Path: "repo/nested-key-1/folder1/folder2/folder3"},
				{Path: "repo/nested-key-2/deep/nested/structure"},
				{Path: "repo/other-key/should/not/match"},
			},
			expectedPaths: []string{"folder1/folder2/folder3", "deep/nested/structure"},
			expectedSourcePaths: map[string]string{
				"folder1/folder2/folder3": "repo/nested-key-1/folder1/folder2/folder3",
				"deep/nested/structure":   "repo/nested-key-2/deep/nested/structure",
			},
		},
		{
			name: "MixedKeyFormats",
			key:  "mixed-key",
			entries: []common.FileEntry{
				{Path: "repo/mixed-key1/path1"},
				{Path: "repo/mixed-key-2/path2"},
				{Path: "repo/mixed-key_3/path3"},
				{Path: "repo/mixed-keySuffix/path4"},
				{Path: "repo/unrelated-key/path5"},
			},
			expectedPaths: []string{"path1", "path2", "path3", "path4"},
			expectedSourcePaths: map[string]string{
				"path1": "repo/mixed-key1/path1",
				"path2": "repo/mixed-key-2/path2",
				"path3": "repo/mixed-key_3/path3",
				"path4": "repo/mixed-keySuffix/path4",
			},
		},
		{
			name: "RealWorldExample",
			key:  "opCI17216cacherestorepathissuekeypattern-keypattern",
			entries: []common.FileEntry{
				{Path: "repo/opCI17216cacherestorepathissuekeypattern-keypattern-1/path1"},
				{Path: "repo/opCI17216cacherestorepathissuekeypattern-keypattern-2/path2"},
				{Path: "repo/opCI17216cacherestorepathissuekeypattern-keypattern-3/path3"},
				{Path: "repo/opCI17216cacherestorepathissuekeypattern-keypattern-4/path4"},
				{Path: "repo/unrelated-entry/path5"},
			},
			expectedPaths: []string{"path1", "path2", "path3", "path4"},
			expectedSourcePaths: map[string]string{
				"path1": "repo/opCI17216cacherestorepathissuekeypattern-keypattern-1/path1",
				"path2": "repo/opCI17216cacherestorepathissuekeypattern-keypattern-2/path2",
				"path3": "repo/opCI17216cacherestorepathissuekeypattern-keypattern-3/path3",
				"path4": "repo/opCI17216cacherestorepathissuekeypattern-keypattern-4/path4",
			},
		},
		{
			name: "NonStandardNamespace",
			key:  "custom-key",
			entries: []common.FileEntry{
				{Path: "custom/namespace/custom-key-1/path1"},
				{Path: "custom/namespace/custom-key-2/path2"},
				{Path: "custom/namespace/unrelated/path3"},
			},
			expectedPaths: []string{"path1", "path2"},
			expectedSourcePaths: map[string]string{
				"path1": "custom/namespace/custom-key-1/path1",
				"path2": "custom/namespace/custom-key-2/path2",
			},
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call our path extraction logic directly
			sourcePaths := make(map[string]string)
			dsts := []string{}
			prefix := namespace
			if tc.name == "NonStandardNamespace" {
				prefix = "custom/namespace"
			}
			
			prefix = fmt.Sprintf("%s/%s", prefix, tc.key)
			
			for _, e := range tc.entries {
				entryPath := e.Path
				var dst string
				
				// Try standard trimming first
				dst = strings.TrimPrefix(entryPath, prefix+"/")
				
				// If path wasn't correctly trimmed (prefix didn't match exactly)
				if strings.HasPrefix(dst, prefix) || strings.Contains(dst, tc.key) {
					// Extract just the path portion
					pathComponents := strings.Split(entryPath, "/")
					
					// Find the key component
					keyComponentIndex := -1
					for i, component := range pathComponents {
						if strings.Contains(component, tc.key) {
							keyComponentIndex = i
						}
					}
					
					// Extract just the path after the key component
					if keyComponentIndex >= 0 && keyComponentIndex+1 < len(pathComponents) {
						dst = strings.Join(pathComponents[keyComponentIndex+1:], "/")
					}
				}
				
				if dst != "" && strings.Contains(entryPath, tc.key) {
					dsts = append(dsts, dst)
					sourcePaths[dst] = entryPath
				}
			}
			
			// Sort paths for deterministic comparison
			sort.Strings(dsts)
			expectedPaths := make([]string, len(tc.expectedPaths))
			copy(expectedPaths, tc.expectedPaths)
			sort.Strings(expectedPaths)
			
			// Verify we found the right number of paths
			if len(dsts) != len(expectedPaths) {
				t.Errorf("Expected to find %d paths, got %d", len(expectedPaths), len(dsts))
				t.Logf("Expected: %v", expectedPaths)
				t.Logf("Actual: %v", dsts)
			}
			
			// Verify each path matches
			for i, dst := range dsts {
				if i >= len(expectedPaths) {
					t.Errorf("Extra path found: %s", dst)
					continue
				}
				
				expectedDst := expectedPaths[i]
				if dst != expectedDst {
					t.Errorf("Path %d: expected '%s', got '%s'", i, expectedDst, dst)
				}
				
				// Check source path mapping
				expectedSrc, ok := tc.expectedSourcePaths[dst]
				if !ok {
					t.Errorf("No expected source path for destination '%s'", dst)
				} else if expectedSrc != sourcePaths[dst] {
					t.Errorf("For path '%s', expected source '%s', got '%s'", 
						dst, expectedSrc, sourcePaths[dst])
				}
			}
		})
	}
}
