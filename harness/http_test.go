package harness

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestGetBackendType(t *testing.T) {
	client := &HTTPClient{}

	// Test case 1: Environment variable not set (should default to s3)
	os.Unsetenv("HARNESS_CI_USE_GCS_CACHE_SERVICE")
	backend := client.getBackendType()
	if backend != "s3" {
		t.Errorf("Expected 's3', got '%s'", backend)
	}

	// Test case 2: Environment variable set to "true" (should return gcs)
	os.Setenv("HARNESS_CI_USE_GCS_CACHE_SERVICE", "true")
	backend = client.getBackendType()
	if backend != "gcs" {
		t.Errorf("Expected 'gcs', got '%s'", backend)
	}

	// Test case 3: Environment variable set to "TRUE" (should return gcs - case insensitive)
	os.Setenv("HARNESS_CI_USE_GCS_CACHE_SERVICE", "TRUE")
	backend = client.getBackendType()
	if backend != "gcs" {
		t.Errorf("Expected 'gcs', got '%s'", backend)
	}

	// Test case 4: Environment variable set to "false" (should return s3)
	os.Setenv("HARNESS_CI_USE_GCS_CACHE_SERVICE", "false")
	backend = client.getBackendType()
	if backend != "s3" {
		t.Errorf("Expected 's3', got '%s'", backend)
	}

	// Cleanup
	os.Unsetenv("HARNESS_CI_USE_GCS_CACHE_SERVICE")
}

func TestGetUploadURLWithBackendParameter(t *testing.T) {
	// Create a test server that returns a simple response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return the request URL so we can verify the backend parameter was added
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("http://presigned-url.example.com"))
	}))
	defer server.Close()

	client := New(server.URL, "test-account", "test-token", false)

	// Test case 1: With GCS backend
	os.Setenv("HARNESS_CI_USE_GCS_CACHE_SERVICE", "true")
	_, err := client.GetUploadURL(context.Background(), "test-key")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Test case 2: With S3 backend
	os.Setenv("HARNESS_CI_USE_GCS_CACHE_SERVICE", "false")
	_, err = client.GetUploadURL(context.Background(), "test-key")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Cleanup
	os.Unsetenv("HARNESS_CI_USE_GCS_CACHE_SERVICE")
}

func TestURLConstruction(t *testing.T) {
	client := &HTTPClient{
		Endpoint:  "https://api.example.com",
		AccountID: "test-account",
	}

	// Test case 1: S3 backend
	os.Setenv("HARNESS_CI_USE_GCS_CACHE_SERVICE", "false")
	path := client.buildEndpointPath(StoreEndpoint, "test-key")
	fullURL := client.Endpoint + path
	
	backend := client.getBackendType()
	if backend != "" {
		if strings.Contains(fullURL, "?") {
			fullURL += "&backend=" + backend
		} else {
			fullURL += "?backend=" + backend
		}
	}

	expectedS3 := "https://api.example.com/cache/intel/upload?accountId=test-account&cacheKey=test-key&backend=s3"
	if fullURL != expectedS3 {
		t.Errorf("Expected '%s', got '%s'", expectedS3, fullURL)
	}

	// Test case 2: GCS backend
	os.Setenv("HARNESS_CI_USE_GCS_CACHE_SERVICE", "true")
	path = client.buildEndpointPath(StoreEndpoint, "test-key")
	fullURL = client.Endpoint + path
	
	backend = client.getBackendType()
	if backend != "" {
		if strings.Contains(fullURL, "?") {
			fullURL += "&backend=" + backend
		} else {
			fullURL += "?backend=" + backend
		}
	}

	expectedGCS := "https://api.example.com/cache/intel/upload?accountId=test-account&cacheKey=test-key&backend=gcs"
	if fullURL != expectedGCS {
		t.Errorf("Expected '%s', got '%s'", expectedGCS, fullURL)
	}

	// Cleanup
	os.Unsetenv("HARNESS_CI_USE_GCS_CACHE_SERVICE")
}
