package harness

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestAddRequestParameters(t *testing.T) {
	t.Setenv("HARNESS_CI_USE_GCS_CACHE_SERVICE", "false")

	client := New("https://api.example.com", "account", "", false)
	client.OrgID = "org with space"
	client.ProjectID = "project"
	client.PipelineID = "pipeline"
	client.StageID = "stage"

	requestURL, err := url.Parse(client.addRequestParameters("https://api.example.com/cache/intel/upload?accountId=account"))
	if err != nil {
		t.Fatal(err)
	}

	query := requestURL.Query()
	if query.Get("backend") != "s3" {
		t.Errorf("backend = %q, want s3", query.Get("backend"))
	}
	if query.Get("orgId") != "org with space" {
		t.Errorf("orgId = %q, want org with space", query.Get("orgId"))
	}
	if query.Get("projectId") != "project" {
		t.Errorf("projectId = %q, want project", query.Get("projectId"))
	}
	if query.Get("pipelineId") != "pipeline" {
		t.Errorf("pipelineId = %q, want pipeline", query.Get("pipelineId"))
	}
	if query.Get("stageId") != "stage" {
		t.Errorf("stageId = %q, want stage", query.Get("stageId"))
	}

	emptyContextURL, err := url.Parse(New("https://api.example.com", "account", "", false).addRequestParameters("https://api.example.com/cache/intel/upload?accountId=account"))
	if err != nil {
		t.Fatal(err)
	}
	if emptyContextURL.Query().Get("orgId") != "" || emptyContextURL.Query().Get("projectId") != "" || emptyContextURL.Query().Get("pipelineId") != "" || emptyContextURL.Query().Get("stageId") != "" {
		t.Errorf("empty context added scope parameters: %s", emptyContextURL.RawQuery)
	}
}

func TestCacheServiceRequestsIncludeHarnessContext(t *testing.T) {
	t.Setenv("HARNESS_CI_USE_GCS_CACHE_SERVICE", "false")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		for key, want := range map[string]string{
			"orgId":      "org",
			"projectId":  "project",
			"pipelineId": "pipeline",
			"stageId":    "stage",
			"backend":    "s3",
		} {
			if got := query.Get(key); got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		}

		if strings.Contains(r.URL.Path, "list_entries") {
			_, _ = w.Write([]byte("[]"))
			return
		}
		_, _ = w.Write([]byte("https://presigned-url.example.com"))
	}))
	defer server.Close()

	client := New(server.URL, "account", "token", false)
	client.OrgID = "org"
	client.ProjectID = "project"
	client.PipelineID = "pipeline"
	client.StageID = "stage"
	ctx := context.Background()

	if _, err := client.GetUploadURLWithQuery(ctx, "cache-key", url.Values{"uploads": {""}}); err != nil {
		t.Fatalf("legacy multipart URL request: %v", err)
	}
	if _, err := client.GetDownloadURLForType(ctx, "step", "cache-key"); err != nil {
		t.Fatalf("unified URL request: %v", err)
	}
	if _, err := client.GetEntriesList(ctx, "cache-prefix"); err != nil {
		t.Fatalf("legacy list request: %v", err)
	}
	if _, err := client.GetEntriesListForType(ctx, "step", "cache-prefix"); err != nil {
		t.Fatalf("unified list request: %v", err)
	}
}
