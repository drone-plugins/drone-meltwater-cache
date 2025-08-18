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

func (m *MockArchive) Extract(dst string, r io.Reader, preserveMetadata bool) (int64, error) {
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
		expectedCorrectPaths map[string]string  // Map of expected correct paths (dst -> src)
	}{
		{
			name:      "ExactKeyMatch",
			namespace: "repo",
			key:       "exact-key-12345",
			entries: []common.FileEntry{
				{Path: "repo/exact-key-12345/path1"},
			},
			expectedCorrectPaths: map[string]string{
				"path1": "repo/exact-key-12345/path1",
			},
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
			expectedCorrectPaths: map[string]string{
				"path1": "repo/prefix-key1/path1",
				"path2": "repo/prefix-key2/path2",
				"path3": "repo/prefix-key3/path3",
			},
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
			expectedCorrectPaths: map[string]string{
				"path1": "repo/prefix-key-1/path1",
				"path2": "repo/prefix-key-2/path2",
				"path3": "repo/prefix-key-3/path3",
			},
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
			expectedCorrectPaths: map[string]string{
				"folder1/folder2/folder3": "repo/nested-key-1/folder1/folder2/folder3",
				"deep/nested/structure":   "repo/nested-key-2/deep/nested/structure",
			},
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
			expectedCorrectPaths: map[string]string{
				"path1": "repo/mixed-key1/path1",
				"path2": "repo/mixed-key-2/path2",
				"path3": "repo/mixed-key_3/path3",
				"path4": "repo/mixed-keySuffix/path4",
			},
		},
		{
			name:      "RealWorldExample",
			namespace: "repo",
			key:       "opCI17216cacherestorepath-keypattern",
			entries: []common.FileEntry{
				{Path: "repo/opCI17216cacherestorepath-keypattern-1/path1"},
				{Path: "repo/opCI17216cacherestorepath-keypattern-2/path2"},
				{Path: "repo/opCI17216cacherestorepath-keypattern-3/path3"},
				{Path: "repo/opCI17216cacherestorepath-keypattern-4/path4"},
				{Path: "repo/unrelated-entry/path5"},
			},
			expectedCorrectPaths: map[string]string{
				"path1": "repo/opCI17216cacherestorepath-keypattern-1/path1",
				"path2": "repo/opCI17216cacherestorepath-keypattern-2/path2",
				"path3": "repo/opCI17216cacherestorepath-keypattern-3/path3",
				"path4": "repo/opCI17216cacherestorepath-keypattern-4/path4",
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
			expectedCorrectPaths: map[string]string{
				"path1": "custom/namespace/custom-key-1/path1",
				"path2": "custom/namespace/custom-key-2/path2",
			},
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
			
			// Build maps of extracted paths and their sources
			extractPathsSeen := make(map[string]bool)
			for _, call := range mockArchive.ExtractCalls {
				extractPathsSeen[call.Dst] = true
			}
			
			getPathsSeen := make(map[string]bool)
			for _, path := range mockStorage.GetCalls {
				getPathsSeen[path] = true
			}
			
			// Check that all the expected paths were correctly processed with the right destination paths
			for expectedDst, expectedSrc := range tc.expectedCorrectPaths {
				if !extractPathsSeen[expectedDst] {
					t.Errorf("Expected path '%s' was not extracted correctly", expectedDst)
				}
				
				if !getPathsSeen[expectedSrc] {
					t.Errorf("Expected source path '%s' was not retrieved", expectedSrc)
				}
			}
			
			// Log useful debug information if requested
			if testing.Verbose() {
				extractPaths := make([]string, 0, len(mockArchive.ExtractCalls))
				for _, call := range mockArchive.ExtractCalls {
					extractPaths = append(extractPaths, call.Dst)
				}
				sort.Strings(extractPaths)
				t.Logf("Extracted paths: %v", extractPaths)
				
				sort.Strings(mockStorage.GetCalls)
				t.Logf("Get calls: %v", mockStorage.GetCalls)
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
	
	// Expected extraction paths after our fix
	expectedPathMapping := map[string]string{
		"path1": "repo/keypattern1/path1",
		"path2": "repo/keypattern-2/path2",
		"path3": "repo/keypattern_3/path3",
		"path4": "repo/keypatternSuffix/path4",
	}
	
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
	
	// Verify correct path extraction
	extractPathsSeen := make(map[string]bool)
	for _, call := range mockArchive.ExtractCalls {
		extractPathsSeen[call.Dst] = true
	}
	
	// Check each expected key->path mapping
	for expectedPath, expectedSrc := range expectedPathMapping {
		// Verify path was extracted correctly
		if !extractPathsSeen[expectedPath] {
			t.Errorf("Expected path '%s' was not extracted correctly", expectedPath)
		}
		
		// Verify source was retrieved
		sourceFound := false
		for _, path := range mockStorage.GetCalls {
			if path == expectedSrc {
				sourceFound = true
				break
			}
		}
		
		if !sourceFound {
			t.Errorf("Expected source path '%s' was not retrieved", expectedSrc)
		}
	}
	
	// Extra debug information
	if testing.Verbose() {
		extractPaths := make([]string, 0, len(mockArchive.ExtractCalls))
		for _, call := range mockArchive.ExtractCalls {
			extractPaths = append(extractPaths, call.Dst)
		}
		sort.Strings(extractPaths)
		t.Logf("All extracted paths: %v", extractPaths)
		
		t.Logf("All Get calls: %v", mockStorage.GetCalls)
	}
}

// Tests the enableCacheKeySeparator option with both true and false settings
// Tests the strictKeyMatching option with both true and false settings
func TestStrictKeyMatchingOptions(t *testing.T) {
	// Define test cases for different key matching settings
	testCases := []struct {
		name              string
		strictKeyMatching bool
		namespace         string
		key               string
		entries           []common.FileEntry
		expectedMatches   []string          // Paths that should be matched
		expectedSkips     []string          // Paths that should be skipped
	}{
		{
			name:              "StrictMatchingEnabled",
			strictKeyMatching: true,
			namespace:         "repo",
			key:               "key-pattern",
			entries: []common.FileEntry{
				{Path: "repo/key-pattern/path1"},           // Exact match - will be processed
				{Path: "repo/key-pattern-1/path2"},         // Not exact match - will be skipped
				{Path: "repo/key-pattern-suffix/path3"},    // Not exact match - will be skipped
			},
			expectedMatches: []string{"path1"},
			expectedSkips:   []string{"repo/key-pattern-1/path2", "repo/key-pattern-suffix/path3"},
		},
		{
			name:              "FlexibleMatchingEnabled",
			strictKeyMatching: false,
			namespace:         "repo",
			key:               "key-pattern",
			entries: []common.FileEntry{
				{Path: "repo/key-pattern/path1"},           // Will be processed
				{Path: "repo/key-pattern-1/path2"},         // Will be processed
				{Path: "repo/key-pattern-suffix/path3"},    // Will be processed
			},
			expectedMatches: []string{"path1", "path2", "path3"},
			expectedSkips:   []string{},
		},
		{
			name:              "MixedPattern",
			strictKeyMatching: true,
			namespace:         "repo",
			key:               "pattern",
			entries: []common.FileEntry{
				{Path: "repo/pattern/path1"},           // Exact match - will be processed
				{Path: "repo/prefix-pattern/path2"},    // Not exact match - will be skipped
				{Path: "repo/pattern-suffix/path3"},   // Not exact match - will be skipped
			},
			expectedMatches: []string{"path1"},
			expectedSkips:   []string{"repo/prefix-pattern/path2", "repo/pattern-suffix/path3"},
		},
		{
			name:              "RealWorldScenario",
			strictKeyMatching: true,
			namespace:         "ns",
			key:               "build-cache-key",
			entries: []common.FileEntry{
				{Path: "ns/build-cache-key/path1"},      // Exact prefix match - will be processed
				{Path: "ns/build-cache-key1/path2"},    // Numeric suffix - should be skipped 
				{Path: "ns/build-cache-key-1/path3"},   // Dash suffix - should be skipped
				{Path: "ns/other-key/path4"},          // Different key - should be skipped
			},
			expectedMatches: []string{"path1"},
			expectedSkips:   []string{"ns/build-cache-key1/path2", "ns/build-cache-key-1/path3", "ns/other-key/path4"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create mocks
			mockStorage := &MockStorage{
				ListFunc: func(p string) ([]common.FileEntry, error) {
					return tc.entries, nil
				},
			}
			
			mockArchive := &MockArchive{}
			
			// Create the restorer with the specified strictKeyMatching setting
			r := restorer{
				logger:                log.NewNopLogger(),
				a:                     mockArchive,
				s:                     mockStorage,
				g:                     generator.NewStatic(tc.key),
				namespace:             tc.namespace,
				strictKeyMatching:     tc.strictKeyMatching,
			}
			
			// Call the Restore method
			err := r.Restore([]string{}, "")
			if err != nil {
				t.Fatalf("Error calling Restore: %v", err)
			}
			
			// Wait for goroutines to complete
			time.Sleep(100 * time.Millisecond)
			
			// Build a map of extracted paths
			extractPaths := make(map[string]bool)
			for _, call := range mockArchive.ExtractCalls {
				extractPaths[call.Dst] = true
			}
			
			// Check that all expected matches were processed
			for _, expectedMatch := range tc.expectedMatches {
				if !extractPaths[expectedMatch] {
					t.Errorf("Expected path '%s' to be processed but it wasn't", expectedMatch)
				}
			}
			
			// Check that expected skips were not processed
			for _, path := range mockStorage.GetCalls {
				for _, skip := range tc.expectedSkips {
					if path == skip {
						t.Errorf("Path '%s' should have been skipped but was processed", skip)
					}
				}
			}
			
			// Log debug info
			if testing.Verbose() {
				var extractedPaths []string
				for _, call := range mockArchive.ExtractCalls {
					extractedPaths = append(extractedPaths, call.Dst)
				}
				t.Logf("Extracted paths: %v", extractedPaths)
				t.Logf("Get calls: %v", mockStorage.GetCalls)
			}
		})
	}
}

func TestCacheKeySeparatorOptions(t *testing.T) {
	// Define test cases for different separator settings
	testCases := []struct {
		name                   string
		enableCacheKeySeparator bool
		namespace              string
		key                    string
		entries                []common.FileEntry
		expectedCorrectPaths   map[string]string
	}{
		{
			name:                   "WithSeparatorEnabled",
			enableCacheKeySeparator: true,
			namespace:              "repo",
			key:                    "key-pattern",
			entries: []common.FileEntry{
				{Path: "repo/key-pattern/path1"},
				{Path: "repo/key-pattern-1/path2"},
				{Path: "repo/key-pattern_3/path3"},
			},
			expectedCorrectPaths: map[string]string{
				"path1": "repo/key-pattern/path1",
				"path2": "repo/key-pattern-1/path2",
				"path3": "repo/key-pattern_3/path3",
			},
		},
		{
			name:                   "WithSeparatorDisabled",
			enableCacheKeySeparator: false,
			namespace:              "repo",
			key:                    "key-pattern",
			entries: []common.FileEntry{
				{Path: "repo/key-pattern/path1"},
				{Path: "repo/key-pattern-1/path2"},
				{Path: "repo/key-pattern_3/path3"},
			},
			expectedCorrectPaths: map[string]string{
				"path1": "repo/key-pattern/path1",
				"path2": "repo/key-pattern-1/path2",
				"path3": "repo/key-pattern_3/path3",
			},
		},
		{
			name:                   "NestedPathsWithSeparator",
			enableCacheKeySeparator: true,
			namespace:              "repo",
			key:                    "nested-key",
			entries: []common.FileEntry{
				{Path: "repo/nested-key/folder1/folder2/folder3"},
				{Path: "repo/nested-key-1/deep/nested/structure"},
			},
			expectedCorrectPaths: map[string]string{
				"folder1/folder2/folder3": "repo/nested-key/folder1/folder2/folder3",
				"deep/nested/structure":   "repo/nested-key-1/deep/nested/structure",
			},
		},
		{
			name:                   "NestedPathsWithoutSeparator",
			enableCacheKeySeparator: false,
			namespace:              "repo",
			key:                    "nested-key",
			entries: []common.FileEntry{
				{Path: "repo/nested-key/folder1/folder2/folder3"},
				{Path: "repo/nested-key-1/deep/nested/structure"},
			},
			expectedCorrectPaths: map[string]string{
				"folder1/folder2/folder3": "repo/nested-key/folder1/folder2/folder3",
				"deep/nested/structure":   "repo/nested-key-1/deep/nested/structure",
			},
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
				logger:                 log.NewNopLogger(),
				a:                      mockArchive,
				s:                      mockStorage,
				g:                      generator.NewStatic(tc.key),
				namespace:              tc.namespace,
				enableCacheKeySeparator: tc.enableCacheKeySeparator,
			}
			
			// Call the actual Restore method
			err := r.Restore([]string{}, "")
			if err != nil {
				t.Fatalf("Error calling Restore: %v", err)
			}
			
			// Wait a reasonable time for all goroutines to complete
			time.Sleep(100 * time.Millisecond)
			
			// Build maps of extracted paths and their sources
			extractPathsSeen := make(map[string]bool)
			for _, call := range mockArchive.ExtractCalls {
				extractPathsSeen[call.Dst] = true
			}
			
			getPathsSeen := make(map[string]bool)
			for _, path := range mockStorage.GetCalls {
				getPathsSeen[path] = true
			}
			
			// Check that all the expected paths were correctly processed
			for expectedDst, expectedSrc := range tc.expectedCorrectPaths {
				if !extractPathsSeen[expectedDst] {
					t.Errorf("Expected path '%s' was not extracted correctly", expectedDst)
				}
				
				if !getPathsSeen[expectedSrc] {
					t.Errorf("Expected source path '%s' was not retrieved", expectedSrc)
				}
			}
			
			// Log useful debug information if requested
			if testing.Verbose() {
				extractPaths := make([]string, 0, len(mockArchive.ExtractCalls))
				for _, call := range mockArchive.ExtractCalls {
					extractPaths = append(extractPaths, call.Dst)
				}
				sort.Strings(extractPaths)
				t.Logf("Extracted paths: %v", extractPaths)
				
				sort.Strings(mockStorage.GetCalls)
				t.Logf("Get calls: %v", mockStorage.GetCalls)
			}
		})
	}
}
