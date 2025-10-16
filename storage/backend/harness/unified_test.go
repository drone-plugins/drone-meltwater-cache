package harness

import (
	"context"
	"io"
	"net/url"
	"strings"
	"testing"

	"github.com/go-kit/kit/log"
	harnessClient "github.com/meltwater/drone-cache/harness"
	"github.com/meltwater/drone-cache/storage/common"
)

// MockUnifiedClient implements both Client and UnifiedClient interfaces
type MockUnifiedClient struct {
	DownloadURLCalls   []string
	UploadURLCalls     []string
	ExistsURLCalls     []string
	ListCalls          []string
	UploadQueryCalls   []string
	UsedUnifiedMethods bool // Track if unified methods were called
}

func (m *MockUnifiedClient) GetUploadURL(ctx context.Context, key string) (string, error) {
	m.UploadURLCalls = append(m.UploadURLCalls, "legacy:"+key)
	return "http://example.com/upload?key=" + key, nil
}

func (m *MockUnifiedClient) GetUploadURLWithQuery(ctx context.Context, key string, query url.Values) (string, error) {
	m.UploadQueryCalls = append(m.UploadQueryCalls, "legacy:"+key)
	return "http://example.com/upload?key=" + key + "&" + query.Encode(), nil
}

func (m *MockUnifiedClient) GetDownloadURL(ctx context.Context, key string) (string, error) {
	m.DownloadURLCalls = append(m.DownloadURLCalls, "legacy:"+key)
	return "http://example.com/download?key=" + key, nil
}

func (m *MockUnifiedClient) GetExistsURL(ctx context.Context, key string) (string, error) {
	m.ExistsURLCalls = append(m.ExistsURLCalls, "legacy:"+key)
	return "http://example.com/exists?key=" + key, nil
}

func (m *MockUnifiedClient) GetEntriesList(ctx context.Context, prefix string) ([]common.FileEntry, error) {
	m.ListCalls = append(m.ListCalls, "legacy:"+prefix)
	return []common.FileEntry{}, nil
}

// Unified methods
func (m *MockUnifiedClient) GetUploadURLForType(ctx context.Context, cacheType string, key string) (string, error) {
	m.UploadURLCalls = append(m.UploadURLCalls, "unified:"+cacheType+":"+key)
	m.UsedUnifiedMethods = true
	return "http://example.com/unified/upload?cacheType=" + cacheType + "&key=" + key, nil
}

func (m *MockUnifiedClient) GetDownloadURLForType(ctx context.Context, cacheType string, key string) (string, error) {
	m.DownloadURLCalls = append(m.DownloadURLCalls, "unified:"+cacheType+":"+key)
	m.UsedUnifiedMethods = true
	return "http://example.com/unified/download?cacheType=" + cacheType + "&key=" + key, nil
}

func (m *MockUnifiedClient) GetExistsURLForType(ctx context.Context, cacheType string, key string) (string, error) {
	m.ExistsURLCalls = append(m.ExistsURLCalls, "unified:"+cacheType+":"+key)
	m.UsedUnifiedMethods = true
	return "http://example.com/unified/exists?cacheType=" + cacheType + "&key=" + key, nil
}

func (m *MockUnifiedClient) GetEntriesListForType(ctx context.Context, cacheType string, prefix string) ([]common.FileEntry, error) {
	m.ListCalls = append(m.ListCalls, "unified:"+cacheType+":"+prefix)
	m.UsedUnifiedMethods = true
	return []common.FileEntry{}, nil
}

func (m *MockUnifiedClient) GetUploadURLWithQueryForType(ctx context.Context, cacheType string, key string, query url.Values) (string, error) {
	m.UploadQueryCalls = append(m.UploadQueryCalls, "unified:"+cacheType+":"+key)
	m.UsedUnifiedMethods = true
	return "http://example.com/unified/upload?cacheType=" + cacheType + "&key=" + key + "&" + query.Encode(), nil
}

// TestUseUnified verifies the useUnified method logic
func TestUseUnified(t *testing.T) {
	testCases := []struct {
		name      string
		cacheType string
		expected  bool
	}{
		{
			name:      "EmptyCacheType",
			cacheType: "",
			expected:  false,
		},
		{
			name:      "WhitespaceCacheType",
			cacheType: "   ",
			expected:  false,
		},
		{
			name:      "ValidCacheType_Step",
			cacheType: "step",
			expected:  true,
		},
		{
			name:      "ValidCacheType_Custom",
			cacheType: "custom-cache",
			expected:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			b := &Backend{
				c: Config{
					CacheType: tc.cacheType,
				},
			}

			result := b.useUnified()
			if result != tc.expected {
				t.Errorf("useUnified() = %v, expected %v for cacheType=%q", result, tc.expected, tc.cacheType)
			}
		})
	}
}

// TestUnifiedClientCasting verifies the unifiedClient method
func TestUnifiedClientCasting(t *testing.T) {
	testCases := []struct {
		name           string
		client         harnessClient.Client
		expectedNotNil bool
	}{
		{
			name:           "UnifiedClient",
			client:         &MockUnifiedClient{},
			expectedNotNil: true,
		},
		{
			name: "LegacyClientOnly",
			client: &struct {
				harnessClient.Client
			}{},
			expectedNotNil: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			b := &Backend{
				client: tc.client,
			}

			uc := b.unifiedClient()
			if tc.expectedNotNil && uc == nil {
				t.Error("Expected unifiedClient to return non-nil, got nil")
			}
			if !tc.expectedNotNil && uc != nil {
				t.Error("Expected unifiedClient to return nil, got non-nil")
			}
		})
	}
}

// TestListUnifiedFlow tests that List calls the correct unified methods
func TestListUnifiedFlow(t *testing.T) {
	testCases := []struct {
		name         string
		cacheType    string
		shouldUseAPI string // "unified" or "legacy"
	}{
		{
			name:         "UnifiedFlow",
			cacheType:    "step",
			shouldUseAPI: "unified",
		},
		{
			name:         "LegacyFlow",
			cacheType:    "",
			shouldUseAPI: "legacy",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &MockUnifiedClient{}
			b := &Backend{
				logger: log.NewNopLogger(),
				client: mockClient,
				c: Config{
					CacheType: tc.cacheType,
				},
			}

			prefix := "test-prefix"
			_, err := b.List(context.Background(), prefix)
			if err != nil {
				t.Fatalf("List failed: %v", err)
			}

			// Verify the correct method was called
			if len(mockClient.ListCalls) != 1 {
				t.Fatalf("Expected 1 List call, got %d", len(mockClient.ListCalls))
			}

			callInfo := mockClient.ListCalls[0]
			if tc.shouldUseAPI == "unified" {
				if !strings.HasPrefix(callInfo, "unified:") {
					t.Errorf("Expected unified API call, got: %s", callInfo)
				}
				if !mockClient.UsedUnifiedMethods {
					t.Error("Expected unified methods to be used")
				}
			} else {
				if !strings.HasPrefix(callInfo, "legacy:") {
					t.Errorf("Expected legacy API call, got: %s", callInfo)
				}
				if mockClient.UsedUnifiedMethods {
					t.Error("Expected legacy methods to be used")
				}
			}
		})
	}
}

// TestGetUnifiedFlow tests Get with unified vs legacy flow
func TestGetUnifiedFlow(t *testing.T) {
	testCases := []struct {
		name         string
		cacheType    string
		shouldUseAPI string
	}{
		{
			name:         "UnifiedFlow",
			cacheType:    "step",
			shouldUseAPI: "unified",
		},
		{
			name:         "LegacyFlow",
			cacheType:    "",
			shouldUseAPI: "legacy",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &MockUnifiedClient{}
			b := &Backend{
				logger: log.NewNopLogger(),
				client: mockClient,
				c: Config{
					CacheType: tc.cacheType,
				},
			}

			// Note: Get will fail because we're not setting up a full HTTP response,
			// but we can check which URL method was called
			key := "test-key"
			_ = b.Get(context.Background(), key, io.Discard)

			// Verify the correct download URL method was called
			if len(mockClient.DownloadURLCalls) == 0 {
				t.Fatal("No download URL calls were made")
			}

			callInfo := mockClient.DownloadURLCalls[0]
			if tc.shouldUseAPI == "unified" {
				if !strings.HasPrefix(callInfo, "unified:") {
					t.Errorf("Expected unified API call, got: %s", callInfo)
				}
				if !strings.Contains(callInfo, tc.cacheType) {
					t.Errorf("Expected call to contain cacheType %q, got: %s", tc.cacheType, callInfo)
				}
			} else {
				if !strings.HasPrefix(callInfo, "legacy:") {
					t.Errorf("Expected legacy API call, got: %s", callInfo)
				}
			}
		})
	}
}

// TestExistsUnifiedFlow tests Exists with unified vs legacy flow
func TestExistsUnifiedFlow(t *testing.T) {
	testCases := []struct {
		name         string
		cacheType    string
		shouldUseAPI string
	}{
		{
			name:         "UnifiedFlow",
			cacheType:    "step",
			shouldUseAPI: "unified",
		},
		{
			name:         "LegacyFlow",
			cacheType:    "",
			shouldUseAPI: "legacy",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &MockUnifiedClient{}
			b := &Backend{
				logger: log.NewNopLogger(),
				client: mockClient,
				c: Config{
					CacheType: tc.cacheType,
				},
			}

			key := "test-key"
			_, _ = b.Exists(context.Background(), key)

			// Verify the correct exists URL method was called
			if len(mockClient.ExistsURLCalls) == 0 {
				t.Fatal("No exists URL calls were made")
			}

			callInfo := mockClient.ExistsURLCalls[0]
			if tc.shouldUseAPI == "unified" {
				if !strings.HasPrefix(callInfo, "unified:") {
					t.Errorf("Expected unified API call, got: %s", callInfo)
				}
				if !strings.Contains(callInfo, tc.cacheType) {
					t.Errorf("Expected call to contain cacheType %q, got: %s", tc.cacheType, callInfo)
				}
			} else {
				if !strings.HasPrefix(callInfo, "legacy:") {
					t.Errorf("Expected legacy API call, got: %s", callInfo)
				}
			}
		})
	}
}

// TestPutUnifiedFlow tests that Put routing uses unified vs legacy correctly
func TestPutUnifiedFlow(t *testing.T) {
	testCases := []struct {
		name         string
		cacheType    string
		shouldUseAPI string
	}{
		{
			name:         "UnifiedFlow",
			cacheType:    "step",
			shouldUseAPI: "unified",
		},
		{
			name:         "LegacyFlow",
			cacheType:    "",
			shouldUseAPI: "legacy",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &MockUnifiedClient{}
			b := &Backend{
				logger: log.NewNopLogger(),
				client: mockClient,
				c: Config{
					CacheType:              tc.cacheType,
					MultipartChunkSize:     512,       // 512MB
					MultipartMaxUploadSize: 50 * 1024, // 50GB
					MultipartThresholdSize: 5 * 1024,  // 5GB
				},
			}

			key := "test-key"
			data := []byte("test data")
			_ = b.Put(context.Background(), key, strings.NewReader(string(data)))

			// Verify the correct upload URL method was called
			if len(mockClient.UploadURLCalls) == 0 {
				t.Fatal("No upload URL calls were made")
			}

			callInfo := mockClient.UploadURLCalls[0]
			if tc.shouldUseAPI == "unified" {
				if !strings.HasPrefix(callInfo, "unified:") {
					t.Errorf("Expected unified API call, got: %s", callInfo)
				}
				if !strings.Contains(callInfo, tc.cacheType) {
					t.Errorf("Expected call to contain cacheType %q, got: %s", tc.cacheType, callInfo)
				}
			} else {
				if !strings.HasPrefix(callInfo, "legacy:") {
					t.Errorf("Expected legacy API call, got: %s", callInfo)
				}
			}
		})
	}
}
