package cache

import (
	"fmt"
	"io"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/meltwater/drone-cache/key/generator"
	"github.com/meltwater/drone-cache/storage/common"
)

// Simple mock objects using functions
type mockStorage struct {
	listFunc    func(p string) ([]common.FileEntry, error)
	getFunc     func(p string, w io.Writer) error
	existsFunc  func(p string) (bool, error)
	putFunc     func(p string, r io.Reader) error
	deleteFunc  func(p string) error
	getCalls    []string
	listCalls   []string
	existsCalls []string
}

func (m *mockStorage) Get(p string, w io.Writer) error {
	if m.getCalls == nil {
		m.getCalls = make([]string, 0)
	}
	m.getCalls = append(m.getCalls, p)
	if m.getFunc != nil {
		return m.getFunc(p, w)
	}
	return nil
}

func (m *mockStorage) Put(p string, r io.Reader) error {
	if m.putFunc != nil {
		return m.putFunc(p, r)
	}
	return nil
}

func (m *mockStorage) Exists(p string) (bool, error) {
	if m.existsCalls == nil {
		m.existsCalls = make([]string, 0)
	}
	m.existsCalls = append(m.existsCalls, p)
	if m.existsFunc != nil {
		return m.existsFunc(p)
	}
	return false, nil
}

func (m *mockStorage) List(p string) ([]common.FileEntry, error) {
	if m.listCalls == nil {
		m.listCalls = make([]string, 0)
	}
	m.listCalls = append(m.listCalls, p)
	if m.listFunc != nil {
		return m.listFunc(p)
	}
	return nil, nil
}

func (m *mockStorage) Delete(p string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(p)
	}
	return nil
}

type mockArchive struct {
	extractFunc func(dst string, r io.Reader) (int64, error)
	createFunc  func(srcs []string, w io.Writer, stripComponents bool) (int64, error)
	extractCalls []string
}

func (m *mockArchive) Create(srcs []string, w io.Writer, stripComponents bool) (int64, error) {
	if m.createFunc != nil {
		return m.createFunc(srcs, w, stripComponents)
	}
	return 0, nil
}

func (m *mockArchive) Extract(dst string, r io.Reader) (int64, error) {
	if m.extractCalls == nil {
		m.extractCalls = make([]string, 0)
	}
	m.extractCalls = append(m.extractCalls, dst)
	if m.extractFunc != nil {
		return m.extractFunc(dst, r)
	}
	return 0, nil
}

func TestRestore(t *testing.T) {
	// Implement me!
}

// Tests for path extraction with various key formats and directory structures
func TestPathExtractionWithDifferentKeyFormats(t *testing.T) {
	// Setup
	namespace := "repo"
	
	// Test cases for different key formats and path structures
	testCases := []struct {
		name               string
		key                string
		entries            []common.FileEntry
		expectedDsts       []string
		expectedSourcePaths map[string]string
	}{
		{
			name: "ExactKeyMatch",
			key:  "exact-key-12345",
			entries: []common.FileEntry{
				{Path: "repo/exact-key-12345/path1"},
			},
			expectedDsts: []string{"path1"},
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
			expectedDsts: []string{"path1", "path2", "path3"},
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
			expectedDsts: []string{"path1", "path2", "path3"},
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
			expectedDsts: []string{"folder1/folder2/folder3", "deep/nested/structure"},
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
			expectedDsts: []string{"path1", "path2", "path3", "path4"},
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
			expectedDsts: []string{"path1", "path2", "path3", "path4"},
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
			expectedDsts: []string{"path1", "path2"},
			expectedSourcePaths: map[string]string{
				"path1": "custom/namespace/custom-key-1/path1",
				"path2": "custom/namespace/custom-key-2/path2",
			},
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockS := &mockStorage{}
			mockA := &mockArchive{}
			
			// Mock storage to return our test entries
			mockS.listFunc = func(p string) ([]common.FileEntry, error) {
				return tc.entries, nil
			}
			
			// Mock archive for extraction
			mockA.extractFunc = func(dst string, r io.Reader) (int64, error) {
				return 0, nil
			}
			
			// Create restorer
			r := restorer{
				logger:                log.NewNopLogger(),
				a:                     mockA,
				s:                     mockS,
				g:                     generator.NewStatic(tc.key),
				namespace:             namespace,
				enableCacheKeySeparator: false,
			}
			
			// Maps to capture actual function calls
			actualSourcePaths := make(map[string]string)
			
			// Create a wrapper around our storage.Get to capture paths
			origGetFunc := mockS.getFunc
			mockS.getFunc = func(p string, w io.Writer) error {
				// For each Get call, find which dst this corresponds to
				for dst, src := range tc.expectedSourcePaths {
					if p == src {
						actualSourcePaths[dst] = p
						break
					}
				}
				// Call the original function if set
				if origGetFunc != nil {
					return origGetFunc(p, w)
				}
				return nil
			}
			
			// Execute the restore
			err := r.Restore([]string{}, "")
			
			// Verify no errors
			if err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
			
			// Verify the number of Get calls matches our expectations
			if len(actualSourcePaths) != len(tc.expectedSourcePaths) {
				t.Errorf("Expected %d paths to be processed, got %d", 
					len(tc.expectedSourcePaths), len(actualSourcePaths))
				fmt.Printf("Expected: %v\n", tc.expectedSourcePaths)
				fmt.Printf("Actual: %v\n", actualSourcePaths)
				fmt.Printf("Get calls: %v\n", mockS.getCalls)
			}
			
			// Verify each expected path was processed with the correct source
			for dst, expectedSrc := range tc.expectedSourcePaths {
				actualSrc, found := actualSourcePaths[dst]
				if !found {
					t.Errorf("Expected destination '%s' was not processed", dst)
				} else if actualSrc != expectedSrc {
					t.Errorf("For destination '%s', expected source '%s', got '%s'", 
						dst, expectedSrc, actualSrc)
				}
			}
		})
	}
}
