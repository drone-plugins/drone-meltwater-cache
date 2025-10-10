package harness

import (
	"context"
	"fmt"
	"net/url"

	"github.com/meltwater/drone-cache/storage/common"
)

// Error is a custom error struct
type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

// Client defines a cache service client.
type Client interface {
	// GetUploadURL returns a presigned URL for uploading a file
	GetUploadURL(ctx context.Context, key string) (string, error)

	// GetDownloadURL returns a presigned URL for downloading a file
	GetDownloadURL(ctx context.Context, key string) (string, error)

	// GetExistsURL returns a presigned URL for checking if a file exists
	GetExistsURL(ctx context.Context, key string) (string, error)

	// GetEntriesList returns a list of files with the given prefix
	GetEntriesList(ctx context.Context, prefix string) ([]common.FileEntry, error)

	// GetUploadURLWithQuery returns a presigned URL for uploading a file with the given query
	GetUploadURLWithQuery(ctx context.Context, key string, query url.Values) (string, error)
}

// UnifiedClient exposes cacheType-aware unified endpoints without changing the legacy Client.
// Implemented by HTTPClient.
type UnifiedClient interface {
	// GetUploadURLForType returns a presigned URL for uploading for a given cacheType.
	GetUploadURLForType(ctx context.Context, cacheType string, key string) (string, error)

	// GetDownloadURLForType returns a presigned URL for downloading for a given cacheType.
	GetDownloadURLForType(ctx context.Context, cacheType string, key string) (string, error)

	// GetExistsURLForType returns a presigned URL for existence check for a given cacheType.
	GetExistsURLForType(ctx context.Context, cacheType string, key string) (string, error)

	// GetEntriesListForType lists entries for a given prefix and cacheType.
	GetEntriesListForType(ctx context.Context, cacheType string, prefix string) ([]common.FileEntry, error)

	// GetUploadURLWithQueryForType returns presigned URL for uploading with extra query params for cacheType.
	GetUploadURLWithQueryForType(ctx context.Context, cacheType string, key string, query url.Values) (string, error)
}
