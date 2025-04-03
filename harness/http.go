package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/meltwater/drone-cache/storage/common"
)

var _ Client = (*HTTPClient)(nil)

const (
	RestoreEndpoint     = "/cache/intel/download?accountId=%s&cacheKey=%s"
	StoreEndpoint       = "/cache/intel/upload?accountId=%s&cacheKey=%s"
	ExistsEndpoint      = "/cache/intel/exists?accountId=%s&cacheKey=%s"
	ListEntriesEndpoint = "/cache/intel/list_entries?accountId=%s&cacheKeyPrefix=%s"
)

// NewHTTPClient returns a new HTTPClient.
func New(endpoint, accountID, bearerToken string, skipverify bool) *HTTPClient {
	endpoint = strings.TrimSuffix(endpoint, "/")
	client := &HTTPClient{
		Endpoint:    endpoint,
		BearerToken: bearerToken,
		AccountID:   accountID,
		Client: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
	return client
}

// HTTPClient provides an http service client.
type HTTPClient struct {
	Client      *http.Client
	Endpoint    string
	AccountID   string
	BearerToken string
}

// getUploadURL will get the 'put' presigned url from cache service
func (c *HTTPClient) GetUploadURL(ctx context.Context, key string) (string, error) {
	path := fmt.Sprintf(StoreEndpoint, c.AccountID, key)
	return c.getLink(ctx, c.Endpoint+path)
}

// GetUploadURLWithQuery will get the 'put' presigned url from cache service with additional query parameters
func (c *HTTPClient) GetUploadURLWithQuery(ctx context.Context, key string, query url.Values) (string, error) {
	path := fmt.Sprintf(StoreEndpoint, c.AccountID, key)
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

// GetDownloadURL will get the 'get' presigned url from cache service
func (c *HTTPClient) GetDownloadURL(ctx context.Context, key string) (string, error) {
	path := fmt.Sprintf(RestoreEndpoint, c.AccountID, key)
	return c.getLink(ctx, c.Endpoint+path)
}

// GetExistsURL will get the 'exists' presigned url from cache service
func (c *HTTPClient) GetExistsURL(ctx context.Context, key string) (string, error) {
	path := fmt.Sprintf(ExistsEndpoint, c.AccountID, key)
	return c.getLink(ctx, c.Endpoint+path)
}

// GetListURL will get the list of all entries
func (c *HTTPClient) GetEntriesList(ctx context.Context, prefix string) ([]common.FileEntry, error) {
	path := fmt.Sprintf(ListEntriesEndpoint, c.AccountID, prefix)
	req, err := http.NewRequestWithContext(ctx, "GET", c.Endpoint+path, nil)
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
