package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/meltwater/drone-cache/storage/common"
)

var _ Client = (*HTTPClient)(nil)

const (
	RestoreEndpoint     = "/cache/intel/download?accountId=%s&cacheKey=%s"
	StoreEndpoint       = "/cache/intel/upload?accountId=%s&cacheKey=%s"
	ExistsEndpoint      = "/cache/intel/exists?accountId=%s&cacheKey=%s"
	ListEntriesEndpoint = "/cache/intel/list_entries?accountId=%s&cacheKeyPrefix=%s"
)

// Unified endpoints (cacheType-aware). These do not embed the cache type in the path
// and instead expect it to be provided as a query parameter.
const (
	UnifiedRestoreEndpoint     = "/cache/unified/download?accountId=%s&cacheKey=%s"
	UnifiedStoreEndpoint       = "/cache/unified/upload?accountId=%s&cacheKey=%s"
	UnifiedExistsEndpoint      = "/cache/unified/exists?accountId=%s&cacheKey=%s"
	UnifiedListEntriesEndpoint = "/cache/unified/list_entries?accountId=%s&cacheKeyPrefix=%s"
)

// NewHTTPClient returns a new HTTPClient.
func New(endpoint, accountID, bearerToken string, skipverify bool) *HTTPClient {
	endpoint = strings.TrimSuffix(endpoint, "/")
	client := &HTTPClient{
		Endpoint:    endpoint,
		BearerToken: bearerToken,
		AccountID:   accountID,
		Logger:      log.NewNopLogger(), // Default no-op logger
		Client: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
	return client
}

// NewWithLogger returns a new HTTPClient with a logger.
func NewWithLogger(endpoint, accountID, bearerToken string, skipverify bool, logger log.Logger) *HTTPClient {
	client := New(endpoint, accountID, bearerToken, skipverify)
	client.Logger = logger
	return client
}

// HTTPClient provides an http service client.
type HTTPClient struct {
	Client      *http.Client
	Endpoint    string
	AccountID   string
	BearerToken string
	Logger      log.Logger
}

// getUploadURL will get the 'put' presigned url from cache service
func (c *HTTPClient) GetUploadURL(ctx context.Context, key string) (string, error) {
	path := c.buildEndpointPath(StoreEndpoint, key)
	return c.getLink(ctx, c.Endpoint+path)
}

// GetUploadURLWithQuery will get the 'put' presigned url from cache service with additional query parameters
func (c *HTTPClient) GetUploadURLWithQuery(ctx context.Context, key string, query url.Values) (string, error) {
	path := c.buildEndpointPath(StoreEndpoint, key)
	fullURL := c.Endpoint + path
	if len(query) > 0 {
		if strings.Contains(fullURL, "?") {
			fullURL += "&" + query.Encode()
		} else {
			fullURL += "?" + query.Encode()
		}
	}

	// Get the presigned URL from the server
	presignedURL, err := c.getLink(ctx, fullURL)
	if err != nil {
		return "", fmt.Errorf("failed to get presigned URL: %w", err)
	}

	return presignedURL, err
}

// GetUploadURLWithQueryForType returns a presigned URL for uploading with additional query parameters for a given cacheType.
func (c *HTTPClient) GetUploadURLWithQueryForType(ctx context.Context, cacheType string, key string, query url.Values) (string, error) {
	path := c.buildEndpointPath(UnifiedStoreEndpoint, key)
	base := c.Endpoint + path

	if query == nil {
		query = url.Values{}
	}
	query.Set("cacheType", cacheType)

	fullURL := base
	if len(query) > 0 {
		if strings.Contains(fullURL, "?") {
			fullURL += "&" + query.Encode()
		} else {
			fullURL += "?" + query.Encode()
		}
	}

	presignedURL, err := c.getLink(ctx, fullURL)
	if err != nil {
		return "", fmt.Errorf("failed to get presigned URL: %w", err)
	}
	return presignedURL, nil
}

// GetDownloadURL will get the 'get' presigned url from cache service
func (c *HTTPClient) GetDownloadURL(ctx context.Context, key string) (string, error) {
	path := c.buildEndpointPath(RestoreEndpoint, key)
	return c.getLink(ctx, c.Endpoint+path)
}

// GetExistsURL will get the 'exists' presigned url from cache service
func (c *HTTPClient) GetExistsURL(ctx context.Context, key string) (string, error) {
	path := c.buildEndpointPath(ExistsEndpoint, key)
	return c.getLink(ctx, c.Endpoint+path)
}

// GetListURL will get the list of all entries
func (c *HTTPClient) GetEntriesList(ctx context.Context, prefix string) ([]common.FileEntry, error) {
	path := c.buildEndpointPath(ListEntriesEndpoint, prefix)
	fullURL := c.Endpoint + path

	// Add backend query parameter based on environment variable
	fullURL = c.addBackendParameter(fullURL)

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, err
	}
	if c.BearerToken != "" {
		req.Header.Add("X-Harness-Token", c.BearerToken)
	}

	resp, err := c.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get list of entries with status %d", resp.StatusCode)
	}
	var entries []common.FileEntry
	err = json.NewDecoder(resp.Body).Decode(&entries)
	if err != nil {
		return nil, err
	}

	return entries, nil
}

// GetUploadURLForType returns a presigned URL for uploading for the given cacheType using unified endpoints.
func (c *HTTPClient) GetUploadURLForType(ctx context.Context, cacheType string, key string) (string, error) {
	path := c.buildEndpointPath(UnifiedStoreEndpoint, key)
	// Append cacheType as query param
	q := url.Values{}
	q.Set("cacheType", cacheType)
	fullURL := c.appendQuery(c.Endpoint+path, q)
	return c.getLink(ctx, fullURL)
}

// GetDownloadURLForType returns a presigned URL for downloading for the given cacheType using unified endpoints.
func (c *HTTPClient) GetDownloadURLForType(ctx context.Context, cacheType string, key string) (string, error) {
	path := c.buildEndpointPath(UnifiedRestoreEndpoint, key)
	q := url.Values{}
	q.Set("cacheType", cacheType)
	fullURL := c.appendQuery(c.Endpoint+path, q)
	return c.getLink(ctx, fullURL)
}

// GetExistsURLForType returns a presigned URL for existence check for the given cacheType using unified endpoints.
func (c *HTTPClient) GetExistsURLForType(ctx context.Context, cacheType string, key string) (string, error) {
	path := c.buildEndpointPath(UnifiedExistsEndpoint, key)
	q := url.Values{}
	q.Set("cacheType", cacheType)
	fullURL := c.appendQuery(c.Endpoint+path, q)
	return c.getLink(ctx, fullURL)
}

// GetEntriesListForType lists entries for a given prefix and cacheType using unified endpoints.
func (c *HTTPClient) GetEntriesListForType(ctx context.Context, cacheType string, prefix string) ([]common.FileEntry, error) {
	path := c.buildEndpointPath(UnifiedListEntriesEndpoint, prefix)
	baseURL := c.Endpoint + path

	// Add cacheType as a query parameter
	q := url.Values{}
	q.Set("cacheType", cacheType)
	fullURL := c.appendQuery(baseURL, q)

	// Add backend query parameter based on environment variable
	fullURL = c.addBackendParameter(fullURL)

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, err
	}
	if c.BearerToken != "" {
		req.Header.Add("X-Harness-Token", c.BearerToken)
	}

	resp, err := c.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get list of entries with status %d", resp.StatusCode)
	}
	var entries []common.FileEntry
	err = json.NewDecoder(resp.Body).Decode(&entries)
	if err != nil {
		return nil, err
	}

	return entries, nil
}

func (c *HTTPClient) getLink(ctx context.Context, path string) (string, error) {
	// Add backend query parameter based on environment variable
	path = c.addBackendParameter(path)

	req, err := http.NewRequestWithContext(ctx, "GET", path, nil)
	if err != nil {
		return "", err
	}
	if c.BearerToken != "" {
		req.Header.Add("X-Harness-Token", c.BearerToken)
	}

	resp, err := c.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get link with status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *HTTPClient) client() *http.Client {
	return c.Client
}

// getBackendType returns the backend type based on environment variable
// Returns "gcs" if HARNESS_CI_USE_GCS_CACHE_SERVICE is set to "true", "s3" otherwise
func (c *HTTPClient) getBackendType() string {
	if strings.ToLower(os.Getenv("HARNESS_CI_USE_GCS_CACHE_SERVICE")) == "true" {
		return "gcs"
	}
	return "s3"
}

// addBackendParameter adds the backend query parameter to the URL
func (c *HTTPClient) addBackendParameter(urlStr string) string {
	backend := c.getBackendType()
	if backend != "" {
		c.Logger.Log("msg", "routing cache request to backend", "backend", backend)
		if strings.Contains(urlStr, "?") {
			return urlStr + "&backend=" + backend
		} else {
			return urlStr + "?backend=" + backend
		}
	}
	return urlStr
}

// appendQuery appends the provided query values to the given URL string.
func (c *HTTPClient) appendQuery(urlStr string, q url.Values) string {
	if len(q) == 0 {
		return urlStr
	}
	if strings.Contains(urlStr, "?") {
		return urlStr + "&" + q.Encode()
	}
	return urlStr + "?" + q.Encode()
}

// buildEndpointPath constructs a properly formatted and URL-encoded API endpoint path
// This centralizes the URL encoding of path components to handle Windows backslashes
func (c *HTTPClient) buildEndpointPath(endpointFormat string, key string) string {
	// URL encode the key to handle Windows backslashes and other special characters
	encodedKey := url.QueryEscape(key)
	return fmt.Sprintf(endpointFormat, c.AccountID, encodedKey)
}
