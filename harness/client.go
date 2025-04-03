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
