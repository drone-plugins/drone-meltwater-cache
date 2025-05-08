package cache

import (
	"io"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/meltwater/drone-cache/key/generator"
	"github.com/meltwater/drone-cache/storage/common"
)

// MockStorage implements storage.Storage for testing
type MockStorage struct {
	ListFunc    func(p string) ([]common.FileEntry, error)
	GetFunc     func(p string, w io.Writer) error
	PutFunc     func(p string, r io.Reader) error
	DeleteFunc  func(p string) error
	ExistsFunc  func(p string) (bool, error)
	// Track calls for verification
	GetCalls    []string
	mu          sync.Mutex // Protect the calls slice during concurrent access
}

func (m *MockStorage) Get(p string, w io.Writer) error {
	m.mu.Lock()
	m.GetCalls = append(m.GetCalls, p)
	m.mu.Unlock()
	
	if m.GetFunc != nil {
		return m.GetFunc(p, w)
	}
	// Default implementation writes some test data
	_, err := w.Write([]byte("test data"))
	return err
}

func (m *MockStorage) Put(p string, r io.Reader) error {
	if m.PutFunc != nil {
		return m.PutFunc(p, r)
	}
	return nil
}

func (m *MockStorage) Delete(p string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(p)
	}
	return nil
}

func (m *MockStorage) List(p string) ([]common.FileEntry, error) {
	if m.ListFunc != nil {
		return m.ListFunc(p)
	}
	return nil, nil
}

func (m *MockStorage) Exists(p string) (bool, error) {
	if m.ExistsFunc != nil {
		return m.ExistsFunc(p)
	}
	return false, nil
}

// MockArchive implements archive.Archive for testing
type MockArchive struct {
	ExtractFunc func(dst string, r io.Reader) (int64, error)
	CreateFunc  func(srcs []string, w io.Writer, stripComponents bool) (int64, error)
	// Track calls for verification
	ExtractCalls []struct {
		Dst string
		Size int64
	}
	mu          sync.Mutex // Protect the calls slice during concurrent access
}

func (m *MockArchive) Extract(dst string, r io.Reader) (int64, error) {
	if m.ExtractFunc != nil {
		size, err := m.ExtractFunc(dst, r)
		
		m.mu.Lock()
		m.ExtractCalls = append(m.ExtractCalls, struct {
			Dst string
			Size int64
		}{dst, size})
		m.mu.Unlock()
		
		return size, err
	}
	
	// Default implementation just records the call
	m.mu.Lock()
	m.ExtractCalls = append(m.ExtractCalls, struct {
		Dst string
		Size int64
	}{dst, 10})
	m.mu.Unlock()
	
	return 10, nil
}

func (m *MockArchive) Create(srcs []string, w io.Writer, stripComponents bool) (int64, error) {
	if m.CreateFunc != nil {
		return m.CreateFunc(srcs, w, stripComponents)
	}
	return 0, nil
}

// Tests that the actual implementation in restorer.go correctly handles different key formats
func TestRestorerWithDifferentKeyFormats(t *testing.T) {
	// Test cases for different key formats and path structures
	testCases := []struct {
		name                 string
		namespace            string
		key                  string
		entries              []common.FileEntry
		expectedExtractPaths []string
		expectedSourcePaths  []string
	}{
		{
			name:      "ExactKeyMatch",
			namespace: "repo",
			key:       "exact-key-12345",
			entries: []common.FileEntry{
				{Path: "repo/exact-key-12345/path1"},
			},
			expectedExtractPaths: []string{"path1"},
			expectedSourcePaths:  []string{"repo/exact-key-12345/path1"},
		},
		{
			name:      "KeyWithNumericSuffix",
			namespace: "repo",
			key:       "prefix-key",
			entries: []common.FileEntry{
				{Path: "repo/prefix-key1/path1"},
				{Path: "repo/prefix-key2/path2"},
				{Path: "repo/prefix-key3/path3"},
				{Path: "repo/other-key/path4"},
			},
			expectedExtractPaths: []string{"path1", "path2", "path3"},
			expectedSourcePaths:  []string{"repo/prefix-key1/path1", "repo/prefix-key2/path2", "repo/prefix-key3/path3"},
		},
		{
			name:      "KeyWithDashSuffix",
			namespace: "repo",
			key:       "prefix-key",
			entries: []common.FileEntry{
				{Path: "repo/prefix-key-1/path1"},
				{Path: "repo/prefix-key-2/path2"},
				{Path: "repo/prefix-key-3/path3"},
				{Path: "repo/other-key/path4"},
			},
			expectedExtractPaths: []string{"path1", "path2", "path3"},
			expectedSourcePaths:  []string{"repo/prefix-key-1/path1", "repo/prefix-key-2/path2", "repo/prefix-key-3/path3"},
		},
		{
			name:      "NestedDirectoryPaths",
			namespace: "repo",
			key:       "nested-key",
			entries: []common.FileEntry{
				{Path: "repo/nested-key-1/folder1/folder2/folder3"},
				{Path: "repo/nested-key-2/deep/nested/structure"},
				{Path: "repo/other-key/should/not/match"},
			},
			expectedExtractPaths: []string{"folder1/folder2/folder3", "deep/nested/structure"},
			expectedSourcePaths:  []string{"repo/nested-key-1/folder1/folder2/folder3", "repo/nested-key-2/deep/nested/structure"},
		},
		{
			name:      "MixedKeyFormats",
			namespace: "repo",
			key:       "mixed-key",
			entries: []common.FileEntry{
				{Path: "repo/mixed-key1/path1"},
				{Path: "repo/mixed-key-2/path2"},
				{Path: "repo/mixed-key_3/path3"},
				{Path: "repo/mixed-keySuffix/path4"},
				{Path: "repo/unrelated-key/path5"},
			},
			expectedExtractPaths: []string{"path1", "path2", "path3", "path4"},
			expectedSourcePaths:  []string{"repo/mixed-key1/path1", "repo/mixed-key-2/path2", "repo/mixed-key_3/path3", "repo/mixed-keySuffix/path4"},
		},
		{
			name:      "RealWorldExample",
			namespace: "repo",
			key:       "opCI17216cacherestorepathissuekeypattern-keypattern",
			entries: []common.FileEntry{
				{Path: "repo/opCI17216cacherestorepathissuekeypattern-keypattern-1/path1"},
				{Path: "repo/opCI17216cacherestorepathissuekeypattern-keypattern-2/path2"},
				{Path: "repo/opCI17216cacherestorepathissuekeypattern-keypattern-3/path3"},
				{Path: "repo/opCI17216cacherestorepathissuekeypattern-keypattern-4/path4"},
				{Path: "repo/unrelated-entry/path5"},
			},
			expectedExtractPaths: []string{"path1", "path2", "path3", "path4"},
			expectedSourcePaths:  []string{
				"repo/opCI17216cacherestorepathissuekeypattern-keypattern-1/path1",
				"repo/opCI17216cacherestorepathissuekeypattern-keypattern-2/path2",
				"repo/opCI17216cacherestorepathissuekeypattern-keypattern-3/path3",
				"repo/opCI17216cacherestorepathissuekeypattern-keypattern-4/path4",
			},
		},
		{
			name:      "NonStandardNamespace",
			namespace: "custom/namespace",
			key:       "custom-key",
			entries: []common.FileEntry{
				{Path: "custom/namespace/custom-key-1/path1"},
				{Path: "custom/namespace/custom-key-2/path2"},
				{Path: "custom/namespace/unrelated/path3"},
			},
			expectedExtractPaths: []string{"path1", "path2"},
			expectedSourcePaths:  []string{"custom/namespace/custom-key-1/path1", "custom/namespace/custom-key-2/path2"},
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create mocks
			mockStorage := &MockStorage{
				ListFunc: func(p string) ([]common.FileEntry, error) {
					return tc.entries, nil
				},
				GetFunc: func(p string, w io.Writer) error {
					// Just write some test data
					_, err := w.Write([]byte("test data"))
					return err
				},
			}
			
			mockArchive := &MockArchive{}
			
			// Create the actual restorer we want to test
			r := restorer{
				logger:                log.NewNopLogger(),
				a:                     mockArchive,
				s:                     mockStorage,
				g:                     generator.NewStatic(tc.key),
				namespace:             tc.namespace,
				enableCacheKeySeparator: false,
			}
			
			// Call the actual Restore method
			err := r.Restore([]string{}, "")
			if err != nil {
				t.Fatalf("Error calling Restore: %v", err)
			}
			
			// Wait a reasonable time for all goroutines to complete
			// This is necessary because Restore launches goroutines to process files
			time.Sleep(100 * time.Millisecond)
			
			// Verify the right paths were processed
			if len(mockArchive.ExtractCalls) != len(tc.expectedExtractPaths) {
				t.Errorf("Expected %d extractions, got %d", 
					len(tc.expectedExtractPaths), len(mockArchive.ExtractCalls))
				
				// Print details for debugging
				extractPaths := make([]string, 0, len(mockArchive.ExtractCalls))
				for _, call := range mockArchive.ExtractCalls {
					extractPaths = append(extractPaths, call.Dst)
				}
				sort.Strings(extractPaths)
				
				expected := make([]string, len(tc.expectedExtractPaths))
				copy(expected, tc.expectedExtractPaths)
				sort.Strings(expected)
				
				t.Logf("Expected extract paths: %v", expected)
				t.Logf("Actual extract paths: %v", extractPaths)
			}
			
			// Verify Get was called with all the expected source paths
			if len(mockStorage.GetCalls) != len(tc.expectedSourcePaths) {
				t.Errorf("Expected %d Get calls, got %d", 
					len(tc.expectedSourcePaths), len(mockStorage.GetCalls))
				
				// Print details for debugging
				sort.Strings(mockStorage.GetCalls)
				
				expected := make([]string, len(tc.expectedSourcePaths))
				copy(expected, tc.expectedSourcePaths)
				sort.Strings(expected)
				
				t.Logf("Expected source paths: %v", expected)
				t.Logf("Actual Get calls: %v", mockStorage.GetCalls)
			}
			
			// Check each expected extraction path was processed
			extractPaths := make(map[string]bool)
			for _, call := range mockArchive.ExtractCalls {
				extractPaths[call.Dst] = true
			}
			
			for _, expectedPath := range tc.expectedExtractPaths {
				if !extractPaths[expectedPath] {
					t.Errorf("Expected path '%s' was not extracted", expectedPath)
				}
			}
			
			// Check each expected source path was retrieved
			getCalls := make(map[string]bool)
			for _, path := range mockStorage.GetCalls {
				getCalls[path] = true
			}
			
			for _, expectedPath := range tc.expectedSourcePaths {
				if !getCalls[expectedPath] {
					t.Errorf("Expected source path '%s' was not retrieved", expectedPath)
				}
			}
		})
	}
}

// This test specifically validates our fix for handling key patterns
func TestFlexibleKeyMatchingRestoration(t *testing.T) {
	// Test case that simulates the real-world scenario
	const namespace = "repo"
	const partialKey = "keypattern"
	
	entries := []common.FileEntry{
		{Path: "repo/keypattern1/path1"},
		{Path: "repo/keypattern-2/path2"},
		{Path: "repo/keypattern_3/path3"},
		{Path: "repo/keypatternSuffix/path4"},
		{Path: "repo/unrelated/path5"},
	}
	
	// Expected extraction paths - just the paths without the key portion
	expectedPaths := []string{"path1", "path2", "path3", "path4"}
	
	// Setup mocks
	mockStorage := &MockStorage{
		ListFunc: func(p string) ([]common.FileEntry, error) {
			return entries, nil
		},
	}
	
	mockArchive := &MockArchive{}
	
	// Create restorer with the partial key
	r := restorer{
		logger:                log.NewNopLogger(),
		a:                     mockArchive,
		s:                     mockStorage,
		g:                     generator.NewStatic(partialKey),
		namespace:             namespace,
		enableCacheKeySeparator: false,
	}
	
	// Call Restore
	err := r.Restore([]string{}, "")
	if err != nil {
		t.Fatalf("Error calling Restore: %v", err)
	}
	
	// Wait a reasonable time for all goroutines to complete
	time.Sleep(100 * time.Millisecond)
	
	// Check that only the expected paths were extracted
	if len(mockArchive.ExtractCalls) != len(expectedPaths) {
		t.Errorf("Expected %d extractions, got %d", 
			len(expectedPaths), len(mockArchive.ExtractCalls))
		
		var extractedPaths []string
		for _, call := range mockArchive.ExtractCalls {
			extractedPaths = append(extractedPaths, call.Dst)
		}
		t.Logf("Expected paths: %v", expectedPaths)
		t.Logf("Actual paths: %v", extractedPaths)
	}
	
	// Verify each path was correctly extracted
	pathsProcessed := make(map[string]bool)
	for _, call := range mockArchive.ExtractCalls {
		pathsProcessed[call.Dst] = true
	}
	
	for _, expected := range expectedPaths {
		if !pathsProcessed[expected] {
			t.Errorf("Expected path '%s' was not processed", expected)
		}
	}
	
	// Check we didn't process the unrelated path
	if pathsProcessed["path5"] {
		t.Error("Unrelated path 'path5' was incorrectly processed")
	}
}
