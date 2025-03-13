package harness

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/meltwater/drone-cache/harness"
	"github.com/meltwater/drone-cache/internal"
	"github.com/meltwater/drone-cache/storage/common"
)

// MultipartUploadInitResponse represents the XML response for multipart upload initiation
type MultipartUploadInitResponse struct {
	XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	UploadID string   `xml:"UploadId"`
}

// CompletedPartElement represents a completed part in the multipart upload
type CompletedPartElement struct {
	PartNumber int    `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
	Key        string // Internal use only, not part of XML
}

// CompleteMultipartUploadRequest represents the XML request for completing multipart upload
type CompleteMultipartUploadRequest struct {
	XMLName xml.Name               `xml:"CompleteMultipartUpload"`
	Parts   []CompletedPartElement `xml:"Part"`
	Checksum string                `xml:"Checksum,omitempty"` // MD5 checksum of the complete file
}

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
	// First try to get the XML file that contains part information
	preSignedURL, err := b.client.GetDownloadURL(ctx, key)
	if err != nil {
		return err
	}

	res, err := b.do(ctx, "GET", preSignedURL, nil)
	if err != nil {
		return err
	}
	defer internal.CloseWithErrLogf(b.logger, res.Body, "response body, close defer")

	// If not found, this might be a regular file
	if res.StatusCode == http.StatusNotFound {
		b.logger.Log("msg", "file not found, checking if it's a multipart file", "key", key)
		return fmt.Errorf("file not found: %s", key)
	}

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("received status code %d from presigned get url", res.StatusCode)
	}

	// Try to parse the response as XML
	var completeReq CompleteMultipartUploadRequest
	if err := xml.NewDecoder(res.Body).Decode(&completeReq); err != nil {
		// If we can't parse as XML, this is a regular file
		b.logger.Log("msg", "not a multipart file, copying directly", "key", key)
		// Reset the body reader to the beginning
		res.Body.Close()
		// Get the file again since we consumed the body
		res, err = b.do(ctx, "GET", preSignedURL, nil)
		if err != nil {
			return err
		}
		defer internal.CloseWithErrLogf(b.logger, res.Body, "response body, close defer")
		_, err = io.Copy(w, res.Body)
		return err
	}

	// This is a multipart file, download and combine all parts
	b.logger.Log(
		"msg", "found multipart file, downloading parts",
		"key", key,
		"numParts", len(completeReq.Parts),
	)

	// Create a temp buffer for each part and calculate checksum
	var combinedData bytes.Buffer
	hash := md5.New()
	mw := io.MultiWriter(&combinedData, hash)

	// Download and combine each part in order
	for _, part := range completeReq.Parts {
		b.logger.Log(
			"msg", "downloading part",
			"partNumber", part.PartNumber,
			"partKey", part.Key,
		)

		// Get the part
		partURL, err := b.client.GetDownloadURL(ctx, part.Key)
		if err != nil {
			return fmt.Errorf("failed to get download URL for part %d: %w", part.PartNumber, err)
		}

		partRes, err := b.do(ctx, "GET", partURL, nil)
		if err != nil {
			return fmt.Errorf("failed to download part %d: %w", part.PartNumber, err)
		}
		defer internal.CloseWithErrLogf(b.logger, partRes.Body, "part response body, close defer")

		if partRes.StatusCode != http.StatusOK {
			return fmt.Errorf("received status code %d when downloading part %d", partRes.StatusCode, part.PartNumber)
		}

		// Copy this part's data to the combined buffer and hash
		_, err = io.Copy(mw, partRes.Body)
		if err != nil {
			return fmt.Errorf("failed to read part %d data: %w", part.PartNumber, err)
		}
	}

	// Calculate final checksum
	restoredChecksum := fmt.Sprintf("%x", hash.Sum(nil))

	// Verify checksum matches
	if completeReq.Checksum != "" && completeReq.Checksum != restoredChecksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", completeReq.Checksum, restoredChecksum)
	}

	b.logger.Log(
		"msg", "successfully combined all parts",
		"key", key,
		"numParts", len(completeReq.Parts),
		"totalSize", combinedData.Len(),
		"checksum", restoredChecksum,
		"checksumMatch", completeReq.Checksum == restoredChecksum,
	)

	// Write the combined data to the output writer
	_, err = io.Copy(w, &combinedData)
	return err
}

const (
	// 5GB in bytes - threshold for multipart upload
	multipartThreshold = 5 * 1024 * 1024 * 1024
	// 64MB chunk size for multipart uploads
	multipartChunkSize = 64 * 1024 * 1024
)

func (b *Backend) Put(ctx context.Context, key string, r io.Reader) error {
	// Clean the key path
	key = strings.TrimPrefix(key, "/")

	// If this is a directory (ends with slash), create an empty marker
	if strings.HasSuffix(key, "/") || strings.HasSuffix(key, "\\") {
		// Ensure the key ends with forward slash
		key = strings.TrimSuffix(key, "\\") + "/"

		// For directories, just create an empty object with trailing slash
		preSignedURL, err := b.client.GetUploadURL(ctx, key)
		if err != nil {
			return err
		}
		res, err := b.do(ctx, "PUT", preSignedURL, strings.NewReader(""))
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

	// Create a buffer to store data and calculate checksum
	buf := &bytes.Buffer{}
	hash := md5.New()
	mw := io.MultiWriter(buf, hash)
	
	// Copy data to buffer while calculating hash
	totalSize, err := io.Copy(mw, r)
	if err != nil {
		return fmt.Errorf("failed to read data: %w", err)
	}
	
	// Get the MD5 checksum
	checksum := fmt.Sprintf("%x", hash.Sum(nil))
	
	// Log the original file checksum
	b.logger.Log(
		"msg", "calculated file checksum",
		"key", key,
		"size", totalSize,
		"checksum", checksum,
	)

	// Ensure we're using forward slashes in the key
	key = strings.ReplaceAll(key, "\\", "/")

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

		// Log the initiation URL for debugging
		b.logger.Log(
			"msg", "generated presigned URL for multipart upload initiation",
			"key", key,
			"url", initiateURL,
		)

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

		// Try to get upload ID from headers first
		uploadID := res.Header.Get("X-Upload-Id")
		if uploadID == "" {
			// If not in headers, try to parse XML response if body is not empty
			body, err := io.ReadAll(res.Body)
			if err != nil {
				return fmt.Errorf("failed to read response body: %w", err)
			}

			// Only try to parse XML if body is not empty
			if len(body) > 0 {
				var initResponse MultipartUploadInitResponse
				if err := xml.Unmarshal(body, &initResponse); err != nil {
					return fmt.Errorf("failed to parse multipart upload initiation response: %w", err)
				}
				uploadID = initResponse.UploadID
			}
		}

		// If still no upload ID, generate one using timestamp (fallback)
		if uploadID == "" {
			uploadID = fmt.Sprintf("%d-%s", time.Now().UnixNano(), key)
		}

		// Upload parts in chunks
		var completedParts []CompletedPartElement
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

			// Create a unique key for this part
			partKey := fmt.Sprintf("%s.part%d", key, partNumber)

			// Get a new presigned URL for uploading this part
			queryParams = url.Values{}
			queryParams.Set("key", partKey) // Use the part-specific key
			queryParams.Set("partNumber", fmt.Sprintf("%d", partNumber))
			queryParams.Set("uploadId", uploadID)
			partURL, err := b.client.GetUploadURLWithQuery(ctx, partKey, queryParams) // Use part-specific key
			if err != nil {
				return err
			}

			// Log the presigned URL and parameters for debugging
			b.logger.Log(
				"msg", "generated presigned URL for part upload",
				"originalKey", key,
				"partKey", partKey,
				"uploadID", uploadID,
				"partNumber", partNumber,
				"url", partURL,
			)

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

			// Store completed part info with the part-specific key
			completedParts = append(completedParts, CompletedPartElement{
				PartNumber: partNumber,
				ETag:       strings.Trim(etag, "\""),
				Key:        partKey,
			})

			partNumber++
			if err == io.ErrUnexpectedEOF {
				break
			}
		}

		// Log the parts we're about to combine
		b.logger.Log(
			"msg", "combining uploaded parts",
			"finalKey", key,
			"uploadID", uploadID,
			"numParts", len(completedParts),
			"parts", func() []string {
				parts := make([]string, len(completedParts))
				for i, part := range completedParts {
					parts[i] = fmt.Sprintf("part%d=%s", part.PartNumber, part.Key)
				}
				return parts
			}(),
		)

		// Complete multipart upload
		completeReq := CompleteMultipartUploadRequest{
			Parts:    completedParts,
			Checksum: checksum,
		}

		// Marshal the completion request to XML
		completeXML, err := xml.Marshal(completeReq)
		if err != nil {
			return fmt.Errorf("failed to marshal completion request: %w", err)
		}

		// Get a new presigned URL for completing the upload
		queryParams = url.Values{}
		queryParams.Set("uploadId", uploadID)
		completeURL, err := b.client.GetUploadURLWithQuery(ctx, key, queryParams)
		if err != nil {
			return err
		}

		// Log completion request
		b.logger.Log(
			"msg", "sending completion request",
			"finalKey", key,
			"uploadID", uploadID,
			"url", completeURL,
		)

		res, err = b.do(ctx, "PUT", completeURL, bytes.NewReader(completeXML))
		if err != nil {
			return fmt.Errorf("failed to complete multipart upload: %w", err)
		}
		defer internal.CloseWithErrLogf(b.logger, res.Body, "response body, close defer")

		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			return fmt.Errorf("failed to complete multipart upload, status code: %d, body: %s", res.StatusCode, string(body))
		}

		// Log successful completion
		b.logger.Log(
			"msg", "multipart upload completed successfully",
			"finalKey", key,
			"uploadID", uploadID,
			"numParts", len(completedParts),
		)

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
