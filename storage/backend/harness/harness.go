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
	"os"
	"sort"
	"strings"
	"sync"
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
	XMLName  xml.Name               `xml:"CompleteMultipartUpload"`
	Parts    []CompletedPartElement `xml:"Part"`
	Checksum string                 `xml:"Checksum,omitempty"` // MD5 checksum of the complete file
}

type Backend struct {
	logger log.Logger
	token  string
	client harness.Client
	c      Config
}

// New creates an Harness backend.
func New(l log.Logger, c Config, debug bool) (*Backend, error) {
	cacheClient := harness.New(c.ServerBaseURL, c.AccountID, c.Token, false)
	backend := &Backend{
		logger: l,
		token:  c.Token,
		client: cacheClient,
		c:      c,
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

	const (
		maxDownloadWorkers = 10 // Maximum number of concurrent downloads
		maxDownloadRetries = 3  // Maximum number of retries per part
		initialBackoff     = 1 * time.Second
	)

	type downloadResult struct {
		partNumber int
		data       []byte
		err        error
	}

	// Create buffered channels for work distribution and result collection
	jobs := make(chan CompletedPartElement, maxDownloadWorkers)
	results := make(chan downloadResult, len(completeReq.Parts))

	// Start worker pool
	var wg sync.WaitGroup
	for w := 0; w < maxDownloadWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for part := range jobs {
				// Worker function to handle part download with retries
				var result downloadResult
				result.partNumber = part.PartNumber
				backoff := initialBackoff

				for attempt := 0; attempt < maxDownloadRetries; attempt++ {
					b.logger.Log(
						"msg", "downloading part",
						"partNumber", part.PartNumber,
						"partKey", part.Key,
						"attempt", attempt+1,
					)

					partURL, err := b.client.GetDownloadURL(ctx, part.Key)
					if err != nil {
						if attempt < maxDownloadRetries-1 {
							time.Sleep(backoff)
							backoff *= 2
							continue
						}
						result.err = fmt.Errorf("failed to get download URL for part %d after %d attempts: %w", part.PartNumber, attempt+1, err)
						break
					}

					partRes, err := b.do(ctx, "GET", partURL, nil)
					if err != nil {
						if attempt < maxDownloadRetries-1 {
							time.Sleep(backoff)
							backoff *= 2
							continue
						}
						result.err = fmt.Errorf("failed to download part %d after %d attempts: %w", part.PartNumber, attempt+1, err)
						break
					}
					defer internal.CloseWithErrLogf(b.logger, partRes.Body, "part response body, close defer")

					if partRes.StatusCode != http.StatusOK {
						if attempt < maxDownloadRetries-1 {
							time.Sleep(backoff)
							backoff *= 2
							continue
						}
						result.err = fmt.Errorf("received status code %d when downloading part %d after %d attempts", partRes.StatusCode, part.PartNumber, attempt+1)
						break
					}

					// Read the entire part into memory
					data, err := io.ReadAll(partRes.Body)
					if err != nil {
						if attempt < maxDownloadRetries-1 {
							time.Sleep(backoff)
							backoff *= 2
							continue
						}
						result.err = fmt.Errorf("failed to read part %d data after %d attempts: %w", part.PartNumber, attempt+1, err)
						break
					}

					// Successfully downloaded the part
					result.data = data
					break // Success, exit retry loop
				}

				results <- result
			}
		}()
	}

	// Distribute parts to workers
	go func() {
		for _, part := range completeReq.Parts {
			jobs <- part
		}
		close(jobs)
	}()

	// Wait for all workers to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect and order results
	downloadedParts := make([]downloadResult, len(completeReq.Parts))
	for result := range results {
		if result.err != nil {
			return result.err
		}
		// Store result in the correct position based on part number
		downloadedParts[result.partNumber-1] = result
	}

	// Create a temp buffer for the combined data and calculate checksum
	var combinedData bytes.Buffer
	hash := md5.New()
	mw := io.MultiWriter(&combinedData, hash)

	// Combine parts in order
	for _, part := range downloadedParts {
		_, err := mw.Write(part.data)
		if err != nil {
			return fmt.Errorf("failed to write part %d to combined buffer: %w", part.partNumber, err)
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

func getMultipartChunkSize(c Config) int64 {
	return int64(c.MultipartChunkSize) * 1024 * 1024 // Convert MB to bytes
}

func getMultipartThresholdSize(c Config) int64 {
	return int64(c.MultipartThresholdSize) * 1024 * 1024 // Convert MB to bytes
}

func getMaxUploadSize(c Config) int64 {
	return int64(c.MultipartMaxUploadSize) * 1024 * 1024 // Convert MB to bytes
}

func enableMultipart(c Config) bool {
	return c.MultipartEnabled == "true"
}

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

	// Calculate total size and checksum
	buf := &bytes.Buffer{}
	hash := md5.New()
	mw := io.MultiWriter(buf, hash)

	// Copy data to buffer while calculating hash
	totalSize, err := io.Copy(mw, r)
	if err != nil {
		return fmt.Errorf("failed to calculate size and checksum: %w", err)
	}

	// Get the MD5 checksum
	checksum := fmt.Sprintf("%x", hash.Sum(nil))

	// Use the buffered data as our reader
	r = buf

	// Check if file size exceeds maximum allowed size
	maxSize := getMaxUploadSize(b.c)
	if totalSize > maxSize {
		return fmt.Errorf("file size %d bytes exceeds maximum allowed size of %d bytes", totalSize, maxSize)
	}

	b.logger.Log(
		"msg", "uploading file",
		"key", key,
		"size", totalSize,
		"checksum", checksum,
	)

	// Log multipart upload configuration
	multipartChunkSize := getMultipartChunkSize(b.c)

	b.logger.Log(
		"msg", "checking multipart upload configuration",
		"PLUGIN_ENABLE_MULTIPART", os.Getenv("PLUGIN_ENABLE_MULTIPART"),
		"Configured Chunk size", multipartChunkSize,
		"Configured max file size", getMaxUploadSize(b.c),
	)

	// Use multipart upload for files larger than 5GB if enabled via env var
	if enableMultipart(b.c) && totalSize > getMultipartThresholdSize(b.c) {
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
		const (
			maxWorkers     = 10 // Maximum number of concurrent uploads
			maxRetries     = 3  // Maximum number of retries per part
			initialBackoff = 1 * time.Second
		)

		type partUploadResult struct {
			part CompletedPartElement
			err  error
		}

		// Create channels for work distribution and result collection
		jobs := make(chan struct {
			partNumber int
			chunk      []byte
		}, maxWorkers)
		results := make(chan partUploadResult, maxWorkers)

		// Start worker pool
		var wg sync.WaitGroup
		for w := 0; w < maxWorkers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for job := range jobs {
					// Worker function to handle part upload with retries
					var result partUploadResult
					backoff := initialBackoff

					for attempt := 0; attempt < maxRetries; attempt++ {
						partKey := fmt.Sprintf("%s.part%d", key, job.partNumber)
						queryParams := url.Values{}
						queryParams.Set("key", partKey)
						queryParams.Set("partNumber", fmt.Sprintf("%d", job.partNumber))
						queryParams.Set("uploadId", uploadID)

						partURL, err := b.client.GetUploadURLWithQuery(ctx, partKey, queryParams)
						if err != nil {
							if attempt < maxRetries-1 {
								time.Sleep(backoff)
								backoff *= 2 // Exponential backoff
								continue
							}
							result.err = fmt.Errorf("failed to get presigned URL for part %d after %d attempts: %w", job.partNumber, attempt+1, err)
							break
						}

						b.logger.Log(
							"msg", "uploading part",
							"originalKey", key,
							"partKey", partKey,
							"uploadID", uploadID,
							"partNumber", job.partNumber,
							"attempt", attempt+1,
						)

						res, err := b.do(ctx, "PUT", partURL, bytes.NewReader(job.chunk))
						if err != nil {
							if attempt < maxRetries-1 {
								time.Sleep(backoff)
								backoff *= 2
								continue
							}
							result.err = fmt.Errorf("failed to upload part %d after %d attempts: %w", job.partNumber, attempt+1, err)
							break
						}

						defer internal.CloseWithErrLogf(b.logger, res.Body, "response body, close defer")

						if res.StatusCode != http.StatusOK {
							body, _ := io.ReadAll(res.Body)
							if attempt < maxRetries-1 {
								time.Sleep(backoff)
								backoff *= 2
								continue
							}
							result.err = fmt.Errorf("received status code %d for part %d after %d attempts, body: %s", res.StatusCode, job.partNumber, attempt+1, string(body))
							break
						}

						etag := res.Header.Get("ETag")
						if etag == "" {
							if attempt < maxRetries-1 {
								time.Sleep(backoff)
								backoff *= 2
								continue
							}
							result.err = fmt.Errorf("no ETag in response for part %d after %d attempts", job.partNumber, attempt+1)
							break
						}

						// Successfully uploaded the part
						result.part = CompletedPartElement{
							PartNumber: job.partNumber,
							ETag:       strings.Trim(etag, "\""),
							Key:        partKey,
						}
						break // Success, exit retry loop
					}

					results <- result
				}
			}()
		}

		// Read and distribute parts to workers
		go func() {
			partNumber := 1
			for {
				chunk := make([]byte, multipartChunkSize)
				n, err := io.ReadFull(r, chunk)
				if err == io.EOF {
					break
				}
				if err != nil && err != io.ErrUnexpectedEOF {
					results <- partUploadResult{err: fmt.Errorf("error reading content for part %d: %w", partNumber, err)}
					break
				}

				jobs <- struct {
					partNumber int
					chunk      []byte
				}{
					partNumber: partNumber,
					chunk:      chunk[:n],
				}

				partNumber++
				if err == io.ErrUnexpectedEOF {
					break
				}
			}
			close(jobs)
		}()

		// Wait for all workers to complete
		go func() {
			wg.Wait()
			close(results)
		}()

		// Collect results and handle errors
		var completedParts []CompletedPartElement
		for result := range results {
			if result.err != nil {
				return result.err
			}
			completedParts = append(completedParts, result.part)
		}

		// Sort completed parts by part number for proper assembly
		sort.Slice(completedParts, func(i, j int) bool {
			return completedParts[i].PartNumber < completedParts[j].PartNumber
		})

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

	httpClient := &http.Client{}
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	return res, nil
}
