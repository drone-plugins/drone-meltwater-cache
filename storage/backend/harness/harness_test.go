//go:build integration
// +build integration

package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/meltwater/drone-cache/storage/common"
)

// MockClient is a mock implementation of the Client interface for testing purposes.
type MockClient struct {
	URL string
}

func (m *MockClient) GetUploadURL(ctx context.Context, key string) (string, error) {
	return m.URL + "?key=" + key, nil
}

func (m *MockClient) GetUploadURLWithQuery(ctx context.Context, key string, query url.Values) (string, error) {
	// Create a new query to avoid modifying the input
	newQuery := url.Values{}
	for k, v := range query {
		newQuery[k] = v
	}
	newQuery.Set("key", key)
	return m.URL + "?" + newQuery.Encode(), nil
}

func (m *MockClient) GetDownloadURL(ctx context.Context, key string) (string, error) {
	return m.URL + "?key=" + key, nil
}

func (m *MockClient) GetExistsURL(ctx context.Context, key string) (string, error) {
	return m.URL + "?key=" + key, nil
}

func (m *MockClient) GetEntriesList(ctx context.Context, key string) ([]common.FileEntry, error) {
	mockEntries := []common.FileEntry{
		{Path: "file1.txt", Size: 1024, LastModified: time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)},
		{Path: "file2.txt", Size: 2048, LastModified: time.Date(2024, 5, 2, 12, 0, 0, 0, time.UTC)},
	}

	return mockEntries, nil
}

func TestGet(t *testing.T) {
	logger := log.NewNopLogger()
	// Create a mock HTTP server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "test data")
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	backend := &Backend{
		logger: logger,
		client: &MockClient{
			URL: server.URL,
		},
	}
	// Execute Get method
	var buf bytes.Buffer
	err := backend.Get(context.Background(), "test-key", &buf)

	// Check for errors
	if err != nil {
		t.Errorf("Get method returned an unexpected error: %v", err)
	}

	// Check the content of the buffer
	expected := "test data"
	if buf.String() != expected {
		t.Errorf("Get method returned unexpected data: got %s, want %s", buf.String(), expected)
	}
}

func TestPut(t *testing.T) {
	logger := log.NewNopLogger()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	backend := &Backend{
		logger: logger,
		client: &MockClient{
			URL: server.URL,
		},
		c: Config{
			MultipartChunkSize:     512,       // 512MB
			MultipartMaxUploadSize: 50 * 1024, // 50GB
			MultipartThresholdSize: 5 * 1024,  // 5GB
		},
	}

	// Execute Put method
	err := backend.Put(context.Background(), "test-key", bytes.NewBuffer([]byte("test data")))

	// Check for errors
	if err != nil {
		t.Errorf("Put method returned an unexpected error: %v", err)
	}
}

func TestExists(t *testing.T) {
	logger := log.NewNopLogger()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("ETag", "test")
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	backend := &Backend{
		logger: logger,
		client: &MockClient{
			URL: server.URL,
		},
	}

	// Execute Exists method
	exists, err := backend.Exists(context.Background(), "test-key")

	// Check for errors
	if err != nil {
		t.Errorf("Exists method returned an unexpected error: %v", err)
	}

	// Check the existence flag
	if !exists {
		t.Error("Exists method returned false, expected true")
	}
}

func TestNotExists(t *testing.T) {
	logger := log.NewNopLogger()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	backend := &Backend{
		logger: logger,
		client: &MockClient{
			URL: server.URL,
		},
	}

	// Execute Exists method
	exists, err := backend.Exists(context.Background(), "test-key")

	// Check for errors
	if err != nil {
		t.Errorf("Exists method returned an unexpected error: %v", err)
	}

	// Check the existence flag
	if exists {
		t.Error("Exists method returned true, expected false")
	}
}

func TestNotExistsWithout404(t *testing.T) {
	logger := log.NewNopLogger()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	backend := &Backend{
		logger: logger,
		client: &MockClient{
			URL: server.URL,
		},
	}

	// Execute Exists method
	exists, err := backend.Exists(context.Background(), "test-key")

	// Check for errors
	if err == nil {
		t.Error("Exists method did not return error")
	}

	// Check the existence flag
	if exists {
		t.Error("Exists method returned true, expected false")
	}
}

func TestList(t *testing.T) {
	logger := log.NewNopLogger()

	mockEntries := []common.FileEntry{
		{Path: "file1.txt", Size: 1024, LastModified: time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)},
		{Path: "file2.txt", Size: 2048, LastModified: time.Date(2024, 5, 2, 12, 0, 0, 0, time.UTC)},
	}
	mockResponseBody, _ := json.Marshal(mockEntries)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(mockResponseBody)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	mockClient := &MockClient{
		URL: server.URL,
	}

	backend := &Backend{
		logger: logger,
		client: mockClient,
	}

	entries, err := backend.List(context.Background(), "test-prefix")

	if err != nil {
		t.Errorf("List method returned an unexpected error: %v", err)
	}

	if len(entries) != len(mockEntries) {
		t.Errorf("List method returned unexpected number of entries: got %d, want %d", len(entries), len(mockEntries))
	}
	for i, entry := range entries {
		if entry.Path != mockEntries[i].Path || entry.Size != mockEntries[i].Size || !entry.LastModified.Equal(mockEntries[i].LastModified) {
			t.Errorf("List method returned unexpected entry at index %d: got %+v, want %+v", i, entry, mockEntries[i])
		}
	}
}

// patternReader implements io.Reader to generate large test data without loading it all into memory
type patternReader struct {
	pattern   []byte
	totalSize int64
	bytesRead int64
}

func (r *patternReader) Read(p []byte) (n int, err error) {
	if r.bytesRead >= r.totalSize {
		return 0, io.EOF
	}

	remaining := r.totalSize - r.bytesRead
	toRead := int64(len(p))
	if toRead > remaining {
		toRead = remaining
	}

	// Fill the output buffer with repeating pattern
	for i := int64(0); i < toRead; i++ {
		p[i] = r.pattern[int((r.bytesRead+i)%int64(len(r.pattern)))]
	}

	r.bytesRead += toRead
	return int(toRead), nil
}

// TestParallelMultipartUploadDownload tests the multipart upload and parallel download functionality
// with a large file that exceeds the multipart threshold.
func TestParallelMultipartUploadDownload(t *testing.T) {
	t.Log("Starting TestParallelMultipartUploadDownload...")
	t.Setenv("PLUGIN_ENABLE_MULTIPART", "true")
	logger := log.NewNopLogger()

	// Track request sequence and upload ID
	uploadID := "test-upload-id"
	partUploads := make(map[int][]byte)
	var uploadedChecksum string
	requestCount := 0

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// All requests should be PUT or GET
		if r.Method != "PUT" && r.Method != "GET" {
			t.Errorf("Expected PUT or GET request, got %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		query := r.URL.Query()
		key := query.Get("key")
		if key == "" {
			t.Error("Missing key parameter")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Handle different types of requests
		switch {
		// Upload Requests
		case r.Method == "PUT" && query.Has("uploads"):
			t.Log("Processing initiate multipart upload request")
			// Return upload ID in XML response
			w.Header().Set("Content-Type", "application/xml")
			initResponse := MultipartUploadInitResponse{
				Bucket:   "test-bucket",
				Key:      key,
				UploadID: uploadID,
			}
			xml.NewEncoder(w).Encode(initResponse)
			requestCount++

		case r.Method == "PUT" && query.Has("partNumber") && query.Get("uploadId") == uploadID:
			t.Logf("Processing part upload request for part %s", query.Get("partNumber"))
			// Read part data
			partData, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("Failed to read part data: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			// Store part data
			partNum := query.Get("partNumber")
			t.Logf("Storing data for part %s, size: %d bytes", partNum, len(partData))
			partNumber := 0
			fmt.Sscanf(partNum, "%d", &partNumber)

			// Store part data
			partUploads[partNumber] = partData

			// Return ETag
			w.Header().Set("ETag", fmt.Sprintf("\"etag-part-%d\"", partNumber))
			w.WriteHeader(http.StatusOK)
			requestCount++

		case r.Method == "PUT" && query.Has("uploadId") && query.Get("uploadId") == uploadID && !query.Has("partNumber"):
			// Parse completion request
			var completeReq CompleteMultipartUploadRequest
			if err := xml.NewDecoder(r.Body).Decode(&completeReq); err != nil {
				t.Errorf("Failed to parse completion request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			// Store the uploaded checksum for verification
			uploadedChecksum = completeReq.Checksum

			// Verify parts are in order
			for i, part := range completeReq.Parts {
				if part.PartNumber != i+1 {
					t.Errorf("Expected part number %d, got %d", i+1, part.PartNumber)
				}
				expectedETag := fmt.Sprintf("\"etag-part-%d\"", part.PartNumber)
				if part.ETag != strings.Trim(expectedETag, "\"") {
					t.Errorf("Expected ETag %s for part %d, got %s", expectedETag, part.PartNumber, part.ETag)
				}
			}

			w.WriteHeader(http.StatusOK)
			requestCount++

		// Download Requests
		case r.Method == "GET" && strings.Contains(key, ".part"):
			// Handle part download request
			partNum := 0
			if _, err := fmt.Sscanf(key, "test-key.part%d", &partNum); err != nil {
				t.Errorf("Failed to parse part number from key %s: %v", key, err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			partData, ok := partUploads[partNum]
			if !ok {
				t.Errorf("Part %d not found", partNum)
				w.WriteHeader(http.StatusNotFound)
				return
			}

			// Return the part data
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(partData)))
			w.WriteHeader(http.StatusOK)
			w.Write(partData)
			requestCount++

		case r.Method == "GET":
			// Return the completion XML for the main file
			var parts []CompletedPartElement
			for i := 1; i <= len(partUploads); i++ {
				parts = append(parts, CompletedPartElement{
					PartNumber: i,
					ETag:       fmt.Sprintf("etag-part-%d", i),
					Key:        fmt.Sprintf("test-key.part%d", i),
				})
			}

			completeReq := CompleteMultipartUploadRequest{
				Parts:    parts,
				Checksum: uploadedChecksum,
			}

			w.Header().Set("Content-Type", "application/xml")
			xml.NewEncoder(w).Encode(completeReq)
			requestCount++

		default:
			t.Errorf("Unexpected request. Method: %s, Path: %s, Query: %v", r.Method, r.URL.Path, query)
			w.WriteHeader(http.StatusBadRequest)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := &MockClient{
		URL: server.URL + "/upload",
	}

	backend := &Backend{
		logger: logger,
		client: client,
		c: Config{
			MultipartChunkSize:     512,       // 512MB
			MultipartMaxUploadSize: 50 * 1024, // 50GB
			MultipartThresholdSize: 5 * 1024,  // 5GB
			MultipartEnabled:       "true",
		},
	}

	// Create test data slightly larger than multipart threshold
	threshold, err := getMultipartThresholdSize(backend.c)
	if err != nil {
		t.Fatalf("Failed to get multipart threshold size: %v", err)
	}
	testDataSize := threshold + 1024*1024 // threshold + 1MB
	t.Logf("Creating test data of size: %d bytes", testDataSize)

	// Create a repeating pattern for test data
	patternSize := 1024 * 1024 // 1MB pattern
	pattern := make([]byte, patternSize)
	for i := range pattern {
		pattern[i] = byte(i % 256)
	}

	// Create a reader that repeats the pattern
	testData := &patternReader{
		pattern:   pattern,
		totalSize: int64(testDataSize),
	}

	// Upload the test data
	t.Log("Starting multipart upload...")
	err = backend.Put(context.Background(), "test-key", testData)
	if err != nil {
		t.Fatalf("Put method failed: %v", err)
	}
	t.Log("Multipart upload completed successfully")

	// Download and verify the data
	t.Log("Starting parallel download...")
	downloadedData := &bytes.Buffer{}
	err = backend.Get(context.Background(), "test-key", downloadedData)
	if err != nil {
		t.Fatalf("Get method failed: %v", err)
	}
	t.Log("Parallel download completed successfully")

	// Verify the downloaded data size
	if downloadedData.Len() != int(testDataSize) {
		t.Errorf("Downloaded data size mismatch: expected %d, got %d", testDataSize, downloadedData.Len())
	}

	// Verify the content pattern
	t.Log("Verifying downloaded data integrity...")
	downloadedBytes := downloadedData.Bytes()
	for i := 0; i < int(testDataSize); i++ {
		expected := pattern[i%patternSize]
		if downloadedBytes[i] != expected {
			t.Errorf("Data mismatch at position %d: expected %d, got %d", i, expected, downloadedBytes[i])
			break
		}
	}

	// Verify request count
	chunkSize, err := getMultipartChunkSize(backend.c)
	if err != nil {
		t.Fatalf("Failed to get multipart chunk size: %v", err)
	}
	expectedParts := (int64(testDataSize) + chunkSize - 1) / chunkSize
	expectedRequests := expectedParts + 2 // initiate + parts + complete
	expectedRequests += expectedParts + 1 // download parts + metadata

	if requestCount != int(expectedRequests) {
		t.Errorf("Expected %d requests, got %d", expectedRequests, requestCount)
	}

	t.Log("Test completed successfully")
}

// TestGetMultipartChunkSize tests the getMultipartChunkSize function with different config values.
func TestGetMultipartChunkSize(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		want    int64
		wantErr bool
	}{
		{
			name: "valid value from config",
			config: Config{
				MultipartChunkSize:     512,       // 512MB
				MultipartMaxUploadSize: 50 * 1024, // 50GB
				MultipartThresholdSize: 5 * 1024,  // 5GB
			},
			want:    512 * 1024 * 1024, // 512MB in bytes
			wantErr: false,
		},
		{
			name: "zero value should error",
			config: Config{
				MultipartChunkSize:     0,         // Invalid
				MultipartMaxUploadSize: 50 * 1024, // 50GB
				MultipartThresholdSize: 5 * 1024,  // 5GB
			},
			want:    0,
			wantErr: true,
		},
		{
			name: "negative value should error",
			config: Config{
				MultipartChunkSize:     -1,        // Invalid
				MultipartMaxUploadSize: 50 * 1024, // 50GB
				MultipartThresholdSize: 5 * 1024,  // 5GB
			},
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getMultipartChunkSize(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("getMultipartChunkSize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getMultipartChunkSize() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGetMaxUploadSize tests the getMaxUploadSize function with different config values.
func TestGetMaxUploadSize(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		want    int64
		wantErr bool
	}{
		{
			name: "valid value from config",
			config: Config{
				MultipartChunkSize:     512,       // 512MB
				MultipartMaxUploadSize: 50 * 1024, // 50GB
				MultipartThresholdSize: 5 * 1024,  // 5GB
			},
			want:    50 * 1024 * 1024 * 1024, // 50GB in bytes
			wantErr: false,
		},
		{
			name: "zero value should error",
			config: Config{
				MultipartChunkSize:     512,  // 512MB
				MultipartMaxUploadSize: 0,    // Invalid
				MultipartThresholdSize: 5120, // 5GB
			},
			want:    0,
			wantErr: true,
		},
		{
			name: "negative value should error",
			config: Config{
				MultipartChunkSize:     512,  // 512MB
				MultipartMaxUploadSize: -1,   // Invalid
				MultipartThresholdSize: 5120, // 5GB
			},
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getMaxUploadSize(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("getMaxUploadSize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getMaxUploadSize() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGetMultipartThresholdSize tests the getMultipartThresholdSize function with different config values.
func TestGetMultipartThresholdSize(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		want    int64
		wantErr bool
	}{
		{
			name: "valid value from config",
			config: Config{
				MultipartChunkSize:     512,       // 512MB
				MultipartMaxUploadSize: 50 * 1024, // 50GB
				MultipartThresholdSize: 5 * 1024,  // 5GB
			},
			want:    5 * 1024 * 1024 * 1024, // 5GB in bytes
			wantErr: false,
		},
		{
			name: "zero value should error",
			config: Config{
				MultipartChunkSize:     512,       // 512MB
				MultipartMaxUploadSize: 50 * 1024, // 50GB
				MultipartThresholdSize: 0,         // Invalid
			},
			want:    0,
			wantErr: true,
		},
		{
			name: "negative value should error",
			config: Config{
				MultipartChunkSize:     512,       // 512MB
				MultipartMaxUploadSize: 50 * 1024, // 50GB
				MultipartThresholdSize: -1,        // Invalid
			},
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getMultipartThresholdSize(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("getMultipartThresholdSize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getMultipartThresholdSize() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPutWithSizeLimit tests the put method with size limit.
func TestPutWithSizeLimit(t *testing.T) {
	logger := log.NewNopLogger()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	tests := []struct {
		name       string
		maxSize    int
		threshold  int
		inputSize  int64
		wantErr    bool
		errMessage string
	}{
		{
			name:      "valid size under limit",
			maxSize:   10,       // 10MB limit
			threshold: 5 * 1024, // 5GB threshold
			inputSize: 5,        // 5MB data
			wantErr:   false,
		},
		{
			name:       "size over limit",
			maxSize:    1,        // 1MB limit
			threshold:  5 * 1024, // 5GB threshold
			inputSize:  2,        // 2MB data
			wantErr:    true,
			errMessage: "exceeds maximum allowed size",
		},
		{
			name:       "invalid max size",
			maxSize:    -1,       // Invalid size
			threshold:  5 * 1024, // 5GB threshold
			inputSize:  1,        // 1MB data
			wantErr:    true,
			errMessage: "invalid multipart max upload size",
		},
		{
			name:       "invalid threshold size",
			maxSize:    10, // 10MB limit
			threshold:  -1, // Invalid threshold
			inputSize:  1,  // 1MB data
			wantErr:    true,
			errMessage: "failed to get multipart threshold size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &Backend{
				logger: logger,
				client: &MockClient{
					URL: server.URL,
				},
				c: Config{
					MultipartChunkSize:     512,          // 512MB
					MultipartMaxUploadSize: tt.maxSize,   // Variable max size
					MultipartThresholdSize: tt.threshold, // Variable threshold
				},
			}

			// Create test data of specified size
			data := make([]byte, tt.inputSize*1024*1024) // Convert MB to bytes

			err := backend.Put(context.Background(), "test-key", bytes.NewBuffer(data))

			if tt.wantErr {
				if err == nil {
					t.Error("Put method should return error")
					return
				}
				if !strings.Contains(err.Error(), tt.errMessage) {
					t.Errorf("Expected error containing %q, got %v", tt.errMessage, err)
				}
			} else if err != nil {
				t.Errorf("Put method returned unexpected error: %v", err)
			}
		})
	}
}

// TestMultipartUploadToggle tests the multipartUploadToggle function with different URL paths.
func TestMultipartUploadToggle(t *testing.T) {
	logger := log.NewNopLogger()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.String(), "uploads=") {
			// This is a multipart upload initiation request
			w.Header().Set("X-Upload-Id", "test-upload-id")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	backend := &Backend{
		logger: logger,
		client: &MockClient{
			URL: server.URL,
		},
		c: Config{
			MultipartChunkSize:     512,       // 512MB
			MultipartMaxUploadSize: 50 * 1024, // 50GB
			MultipartThresholdSize: 5 * 1024,  // 5GB
		},
	}

	// Create data larger than multipart threshold
	chunkSize, err := getMultipartChunkSize(backend.c)
	if err != nil {
		t.Fatalf("Failed to get multipart chunk size: %v", err)
	}
	largeData := make([]byte, chunkSize+1)

	// Test with multipart upload disabled
	// t.Setenv("PLUGIN_ENABLE_MULTIPART", "false")
	err = backend.Put(context.Background(), "test-key", bytes.NewBuffer(largeData))
	if err != nil {
		t.Errorf("Put method returned unexpected error with multipart disabled: %v", err)
	}

	// Test with multipart upload enabled
	// t.Setenv("PLUGIN_ENABLE_MULTIPART", "true")
	err = backend.Put(context.Background(), "test-key", bytes.NewBuffer(largeData))
	if err != nil {
		t.Errorf("Put method returned unexpected error with multipart enabled: %v", err)
	}
}
