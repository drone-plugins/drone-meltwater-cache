package harness

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/meltwater/drone-cache/harness"
	"github.com/meltwater/drone-cache/internal"
	"github.com/meltwater/drone-cache/storage/common"
)

type Backend struct {
	logger log.Logger
	token  string
	client harness.Client
}

// New creates an Harness backend.
func New(l log.Logger, c Config, debug bool) (*Backend, error) {
	cacheClient := harness.New(c.ServerBaseURL, c.AccountID, c.Token, false)
	backend := &Backend{
		logger: l,
		token:  c.Token,
		client: cacheClient,
	}
	return backend, nil
}

func (b *Backend) Get(ctx context.Context, key string, w io.Writer) error {
	preSignedURL, err := b.client.GetDownloadURL(ctx, key)
	if err != nil {
		return err
	}
	res, err := b.do(ctx, "GET", preSignedURL, nil)
	if err != nil {
		return err
	}
	defer internal.CloseWithErrLogf(b.logger, res.Body, "response body, close defer")
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("received status code %d from presigned get url", res.StatusCode)
	}
	_, err = io.Copy(w, res.Body)
	if err != nil {
		return err
	}

	return nil
}

const (
	// 5GB in bytes - threshold for multipart upload
	multipartThreshold = 5 * 1024 * 1024 * 1024
	// 64MB chunk size for multipart uploads
	multipartChunkSize = 64 * 1024 * 1024
)

func (b *Backend) Put(ctx context.Context, key string, r io.Reader) error {
	// Create a buffer to store data and determine size
	buf := &bytes.Buffer{}
	// Copy data to buffer while counting size
	totalSize, err := io.Copy(buf, r)
	if err != nil {
		return fmt.Errorf("failed to read data: %w", err)
	}

	// Use the buffered data as our reader
	r = buf

	// Use multipart upload for files larger than 5GB
	if totalSize > multipartThreshold {
		// Get a new presigned URL for initiating multipart upload
		queryParams := url.Values{}
		queryParams.Set("key", key)
		queryParams.Set("uploads", "")

		initiateURL, err := b.client.GetUploadURLWithQuery(ctx, key, queryParams)
		if err != nil {
			return err
		}

		// Initiate multipart upload (using PUT with ?uploads query param)
		res, err := b.do(ctx, "PUT", initiateURL, nil)
		if err != nil {
			return fmt.Errorf("failed to initiate multipart upload: %w", err)
		}
		defer internal.CloseWithErrLogf(b.logger, res.Body, "response body, close defer")

		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			return fmt.Errorf("failed to initiate multipart upload, status code: %d, body: %s", res.StatusCode, string(body))
		}

		// For S3 PUT operations, a 200 OK with empty body is valid
		// Get upload ID from response headers
		// Try different header fields that might contain the upload ID
		uploadID := res.Header.Get("X-Amz-Version-Id")
		if uploadID == "" {
			uploadID = res.Header.Get("X-Amz-Request-Id")
		}
		if uploadID == "" {
			uploadID = res.Header.Get("ETag")
			// Remove quotes from ETag if present
			uploadID = strings.Trim(uploadID, `"'`)
		}
		if uploadID == "" {
			return fmt.Errorf("no upload ID found in response headers")
		}

		// Upload parts in chunks
		var completedParts []string
		partNumber := 1

		for {
			chunk := make([]byte, multipartChunkSize)
			n, err := io.ReadFull(r, chunk)
			if err == io.EOF {
				break
			}
			if err != nil && err != io.ErrUnexpectedEOF {
				return fmt.Errorf("error reading content for part %d: %w", partNumber, err)
			}

			// Get a new presigned URL for uploading this part
			queryParams = url.Values{}
			queryParams.Set("key", key)
			queryParams.Set("partNumber", fmt.Sprintf("%d", partNumber))
			queryParams.Set("uploadId", uploadID)
			partURL, err := b.client.GetUploadURLWithQuery(ctx, key, queryParams)
			if err != nil {
				return err
			}

			// Upload part directly with PUT
			res, err := b.do(ctx, "PUT", partURL, bytes.NewReader(chunk[:n]))
			if err != nil {
				return fmt.Errorf("failed to upload part %d: %w", partNumber, err)
			}
			defer internal.CloseWithErrLogf(b.logger, res.Body, "response body, close defer")

			if res.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(res.Body)
				return fmt.Errorf("received status code %d for part %d, body: %s", res.StatusCode, partNumber, string(body))
			}

			etag := res.Header.Get("ETag")
			if etag == "" {
				return fmt.Errorf("no ETag in response for part %d", partNumber)
			}
			completedParts = append(completedParts, fmt.Sprintf("<Part><PartNumber>%d</PartNumber><ETag>%s</ETag></Part>", partNumber, etag))

			partNumber++
			if err == io.ErrUnexpectedEOF {
				break
			}
		}

		// Complete multipart upload
		completeXML := fmt.Sprintf("<CompleteMultipartUpload>%s</CompleteMultipartUpload>", strings.Join(completedParts, ""))

		// Get a new presigned URL for completing the upload
		queryParams = url.Values{}
		queryParams.Set("uploadId", uploadID)
		completeURL, err := b.client.GetUploadURLWithQuery(ctx, key, queryParams)
		if err != nil {
			return err
		}

		res, err = b.do(ctx, "PUT", completeURL, strings.NewReader(completeXML))
		if err != nil {
			return fmt.Errorf("failed to complete multipart upload: %w", err)
		}
		defer internal.CloseWithErrLogf(b.logger, res.Body, "response body, close defer")

		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			return fmt.Errorf("failed to complete multipart upload, status code: %d, body: %s", res.StatusCode, string(body))
		}

		return nil
	} else {
		// Get regular upload URL
		preSignedURL, err := b.client.GetUploadURL(ctx, key)
		if err != nil {
			return err
		}
		res, err := b.do(ctx, "PUT", preSignedURL, r)
		if err != nil {
			return err
		}
		defer internal.CloseWithErrLogf(b.logger, res.Body, "response body, close defer")
		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			return fmt.Errorf("received status code %d from presigned put url, body: %s", res.StatusCode, string(body))
		}

		return nil
	}
}

func (b *Backend) Exists(ctx context.Context, key string) (bool, error) {
	preSignedURL, err := b.client.GetExistsURL(ctx, key)
	if err != nil {
		return false, err
	}
	res, err := b.do(ctx, "HEAD", preSignedURL, nil)
	if err != nil {
		return false, nil
	}
	defer internal.CloseWithErrLogf(b.logger, res.Body, "response body, close defer")
	if res.StatusCode == http.StatusNotFound {
		return false, nil
	} else if res.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status code %d", res.StatusCode)
	}

	return res.Header.Get("ETag") != "", nil
}

func (b *Backend) List(ctx context.Context, prefix string) ([]common.FileEntry, error) {
	entries, err := b.client.GetEntriesList(ctx, prefix)
	return entries, err
}

type ListBucketResult struct {
	XMLName               xml.Name  `xml:"ListBucketResult"`
	Contents              []Content `xml:"Contents"`
	IsTruncated           bool      `xml:"IsTruncated"`
	NextContinuationToken string    `xml:"NextContinuationToken"`
}

type Content struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	Size         int64  `xml:"Size"`
}

func (b *Backend) do(ctx context.Context, method, urlStr string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, urlStr, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if !strings.Contains(urlStr, "X-Amz-Signature=") {
		// Only add AWS headers for non-presigned URLs
		req.Header.Set("X-Amz-Content-SHA256", "UNSIGNED-PAYLOAD")
	}

	httpClient := &http.Client{}
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	return res, nil
}
