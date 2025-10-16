package cache

import (
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/meltwater/drone-cache/key/generator"
	"github.com/meltwater/drone-cache/storage/common"
)

// MockStorage implements storage.Storage for testing
type MockStorage struct {
	ListFunc   func(p string) ([]common.FileEntry, error)
	GetFunc    func(p string, w io.Writer) error
	PutFunc    func(p string, r io.Reader) error
	DeleteFunc func(p string) error
	ExistsFunc func(p string) (bool, error)
	// Track calls for verification
	GetCalls []string
	mu       sync.Mutex // Protect the calls slice during concurrent access
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
		Dst  string
		Size int64
	}
	mu sync.Mutex // Protect the calls slice during concurrent access
}

func (m *MockArchive) Extract(dst string, r io.Reader) (int64, error) {
	if m.ExtractFunc != nil {
		size, err := m.ExtractFunc(dst, r)

		m.mu.Lock()
		m.ExtractCalls = append(m.ExtractCalls, struct {
			Dst  string
			Size int64
		}{dst, size})
		m.mu.Unlock()

		return size, err
	}

	// Default implementation just records the call
	m.mu.Lock()
	m.ExtractCalls = append(m.ExtractCalls, struct {
		Dst  string
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
		expectedCorrectPaths map[string]string // Map of expected correct paths (dst -> src)
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
				logger:                  log.NewNopLogger(),
				a:                       mockArchive,
				s:                       mockStorage,
				g:                       generator.NewStatic(tc.key),
				namespace:               tc.namespace,
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
		logger:                  log.NewNopLogger(),
		a:                       mockArchive,
		s:                       mockStorage,
		g:                       generator.NewStatic(partialKey),
		namespace:               namespace,
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
		expectedMatches   []string // Paths that should be matched
		expectedSkips     []string // Paths that should be skipped
	}{
		{
			name:              "StrictMatchingEnabled",
			strictKeyMatching: true,
			namespace:         "repo",
			key:               "key-pattern",
			entries: []common.FileEntry{
				{Path: "repo/key-pattern/path1"},        // Exact match - will be processed
				{Path: "repo/key-pattern-1/path2"},      // Not exact match - will be skipped
				{Path: "repo/key-pattern-suffix/path3"}, // Not exact match - will be skipped
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
				{Path: "repo/key-pattern/path1"},        // Will be processed
				{Path: "repo/key-pattern-1/path2"},      // Will be processed
				{Path: "repo/key-pattern-suffix/path3"}, // Will be processed
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
				{Path: "repo/pattern/path1"},        // Exact match - will be processed
				{Path: "repo/prefix-pattern/path2"}, // Not exact match - will be skipped
				{Path: "repo/pattern-suffix/path3"}, // Not exact match - will be skipped
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
				{Path: "ns/build-cache-key/path1"},   // Exact prefix match - will be processed
				{Path: "ns/build-cache-key1/path2"},  // Numeric suffix - should be skipped
				{Path: "ns/build-cache-key-1/path3"}, // Dash suffix - should be skipped
				{Path: "ns/other-key/path4"},         // Different key - should be skipped
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
				logger:            log.NewNopLogger(),
				a:                 mockArchive,
				s:                 mockStorage,
				g:                 generator.NewStatic(tc.key),
				namespace:         tc.namespace,
				strictKeyMatching: tc.strictKeyMatching,
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
		name                    string
		enableCacheKeySeparator bool
		namespace               string
		key                     string
		entries                 []common.FileEntry
		expectedCorrectPaths    map[string]string
	}{
		{
			name:                    "WithSeparatorEnabled",
			enableCacheKeySeparator: true,
			namespace:               "repo",
			key:                     "key-pattern",
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
			name:                    "WithSeparatorDisabled",
			enableCacheKeySeparator: false,
			namespace:               "repo",
			key:                     "key-pattern",
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
			name:                    "NestedPathsWithSeparator",
			enableCacheKeySeparator: true,
			namespace:               "repo",
			key:                     "nested-key",
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
			name:                    "NestedPathsWithoutSeparator",
			enableCacheKeySeparator: false,
			namespace:               "repo",
			key:                     "nested-key",
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
				logger:                  log.NewNopLogger(),
				a:                       mockArchive,
				s:                       mockStorage,
				g:                       generator.NewStatic(tc.key),
				namespace:               tc.namespace,
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

// TestExportMetricsIfUnified tests that metrics are only exported for unified cache flow
func TestExportMetricsIfUnified(t *testing.T) {
	testCases := []struct {
		name         string
		cacheType    string
		shouldExport bool
	}{
		{
			name:         "UnifiedFlowEnabled",
			cacheType:    "step",
			shouldExport: true,
		},
		{
			name:         "LegacyFlow_EmptyCacheType",
			cacheType:    "",
			shouldExport: false,
		},
		{
			name:         "LegacyFlow_WhitespaceCacheType",
			cacheType:    "   ",
			shouldExport: false,
		},
		{
			name:         "UnifiedFlow_CustomType",
			cacheType:    "custom-cache",
			shouldExport: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up DRONE_OUTPUT for testing
			tmpFile := t.TempDir() + "/output.txt"
			t.Setenv("DRONE_OUTPUT", tmpFile)

			r := restorer{
				logger:    log.NewNopLogger(),
				cacheType: tc.cacheType,
			}

			metrics := map[string]string{
				"cache_hit":   "true",
				"cache_state": "complete",
			}

			// Call the function
			r.exportMetricsIfUnified(metrics)

			// Check if file was created
			_, err := os.Stat(tmpFile)
			fileExists := err == nil

			if tc.shouldExport && !fileExists {
				t.Errorf("Expected metrics to be exported for cacheType=%q, but file was not created", tc.cacheType)
			}

			if !tc.shouldExport && fileExists {
				t.Errorf("Expected metrics NOT to be exported for cacheType=%q, but file was created", tc.cacheType)
			}
		})
	}
}

// TestExportRestoreMetrics tests the export metrics functionality
func TestExportRestoreMetrics(t *testing.T) {
	testCases := []struct {
		name          string
		metrics       map[string]string
		expectError   bool
		expectedLines []string
	}{
		{
			name: "ValidMetrics",
			metrics: map[string]string{
				"cache_hit":   "true",
				"cache_state": "complete",
			},
			expectError: false,
			expectedLines: []string{
				"cache_hit=true",
				"cache_state=complete",
			},
		},
		{
			name: "SingleMetric",
			metrics: map[string]string{
				"cache_hit": "false",
			},
			expectError: false,
			expectedLines: []string{
				"cache_hit=false",
			},
		},
		{
			name: "MetricsWithEmptyValues",
			metrics: map[string]string{
				"cache_hit":   "true",
				"cache_state": "",
			},
			expectError: false,
			expectedLines: []string{
				"cache_hit=true",
				"cache_state=",
			},
		},
		{
			name:          "EmptyMetrics",
			metrics:       map[string]string{},
			expectError:   false,
			expectedLines: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a temporary file for testing
			tmpFile := t.TempDir() + "/output.txt"
			t.Setenv("DRONE_OUTPUT", tmpFile)

			logger := log.NewNopLogger()

			// Call the function
			err := exportRestoreMetrics(logger, tc.metrics)

			// Check error expectation
			if tc.expectError && err == nil {
				t.Error("Expected an error but got none")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			// If no error expected, verify file contents
			if !tc.expectError {
				content, err := os.ReadFile(tmpFile)
				if err != nil {
					t.Fatalf("Failed to read output file: %v", err)
				}

				fileContent := string(content)
				for _, expectedLine := range tc.expectedLines {
					if !strings.Contains(fileContent, expectedLine) {
						t.Errorf("Expected line %q not found in output file. Content: %s", expectedLine, fileContent)
					}
				}
			}
		})
	}
}

// TestExportRestoreMetrics_NoEnvVar tests behavior when DRONE_OUTPUT is not set
func TestExportRestoreMetrics_NoEnvVar(t *testing.T) {
	// Ensure DRONE_OUTPUT is not set
	t.Setenv("DRONE_OUTPUT", "")

	logger := log.NewNopLogger()
	metrics := map[string]string{
		"cache_hit": "true",
	}

	err := exportRestoreMetrics(logger, metrics)
	if err == nil {
		t.Error("Expected error when DRONE_OUTPUT is not set, but got none")
	}

	if !strings.Contains(err.Error(), "DRONE_OUTPUT") {
		t.Errorf("Expected error message to mention DRONE_OUTPUT, got: %v", err)
	}
}

// TestExportRestoreMetrics_AppendBehavior tests that metrics are appended to existing file
func TestExportRestoreMetrics_AppendBehavior(t *testing.T) {
	tmpFile := t.TempDir() + "/output.txt"
	t.Setenv("DRONE_OUTPUT", tmpFile)

	logger := log.NewNopLogger()

	// Write first set of metrics
	metrics1 := map[string]string{
		"first_metric": "value1",
	}
	err := exportRestoreMetrics(logger, metrics1)
	if err != nil {
		t.Fatalf("Failed to write first metrics: %v", err)
	}

	// Write second set of metrics
	metrics2 := map[string]string{
		"second_metric": "value2",
	}
	err = exportRestoreMetrics(logger, metrics2)
	if err != nil {
		t.Fatalf("Failed to write second metrics: %v", err)
	}

	// Read the file and verify both sets are present
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	fileContent := string(content)

	// Check that both metrics are present
	if !strings.Contains(fileContent, "first_metric=value1") {
		t.Error("First metric not found in file")
	}
	if !strings.Contains(fileContent, "second_metric=value2") {
		t.Error("Second metric not found in file")
	}

	// Verify order (first should come before second)
	idx1 := strings.Index(fileContent, "first_metric=value1")
	idx2 := strings.Index(fileContent, "second_metric=value2")
	if idx1 >= idx2 {
		t.Error("Metrics were not appended in correct order")
	}
}

// TestExportRestoreMetrics_SkipEmptyKeys tests that empty keys are skipped
func TestExportRestoreMetrics_SkipEmptyKeys(t *testing.T) {
	tmpFile := t.TempDir() + "/output.txt"
	t.Setenv("DRONE_OUTPUT", tmpFile)

	logger := log.NewNopLogger()

	metrics := map[string]string{
		"valid_key": "value1",
		"":          "should_be_skipped",
		"another":   "value2",
	}

	err := exportRestoreMetrics(logger, metrics)
	if err != nil {
		t.Fatalf("Failed to write metrics: %v", err)
	}

	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	fileContent := string(content)

	// Verify valid keys are present
	if !strings.Contains(fileContent, "valid_key=value1") {
		t.Error("valid_key not found in file")
	}
	if !strings.Contains(fileContent, "another=value2") {
		t.Error("another key not found in file")
	}

	// Verify empty key was skipped
	if strings.Contains(fileContent, "=should_be_skipped") {
		t.Error("Empty key was not skipped")
	}
}

// TestExportRestoreMetrics_ConcurrentWrites tests thread safety
func TestExportRestoreMetrics_ConcurrentWrites(t *testing.T) {
	tmpFile := t.TempDir() + "/output.txt"
	t.Setenv("DRONE_OUTPUT", tmpFile)

	logger := log.NewNopLogger()

	// Write multiple metrics concurrently
	const numGoroutines = 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			metrics := map[string]string{
				"metric": string(rune('A' + index)),
			}
			_ = exportRestoreMetrics(logger, metrics)
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify file was created and has content
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	if len(content) == 0 {
		t.Error("File should have content after concurrent writes")
	}

	// Count the number of lines (should be numGoroutines)
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != numGoroutines {
		t.Errorf("Expected %d lines, got %d", numGoroutines, len(lines))
	}
}
