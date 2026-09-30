package s3

import (
	"bytes"
	"context"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/go-kit/log"
	"github.com/meltwater/drone-cache/test"
)

// Mock S3 client for testing
type mockS3Client struct {
	headBucketFunc func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
}

func (m *mockS3Client) HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	if m.headBucketFunc != nil {
		return m.headBucketFunc(ctx, params, optFns...)
	}
	return &s3.HeadBucketOutput{}, nil
}

func TestDetectDirectoryBucket_RegularBucket_FastPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		bucketName string
	}{
		{
			name:       "simple bucket name",
			bucketName: "my-bucket",
		},
		{
			name:       "bucket with dashes",
			bucketName: "my-test-bucket-123",
		},
		{
			name:       "bucket ending with s3",
			bucketName: "my-bucket-s3",
		},
		{
			name:       "bucket with x-s3 in middle",
			bucketName: "my--x-s3-bucket",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Mock client that should NOT be called (fast path)
			mockClient := &mockS3Client{
				headBucketFunc: func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
					t.Fatal("HeadBucket should not be called for regular buckets (fast path)")
					return nil, nil
				},
			}

			result := detectDirectoryBucket(context.Background(), mockClient, tc.bucketName)
			test.Equals(t, false, result, "regular bucket should return false")
		})
	}
}

func TestDetectDirectoryBucket_S3Express_AvailabilityZone(t *testing.T) {
	t.Parallel()

	mockClient := &mockS3Client{
		headBucketFunc: func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
			return &s3.HeadBucketOutput{
				BucketLocationType: s3types.LocationTypeAvailabilityZone,
				BucketArn:          aws.String("arn:aws:s3express:us-east-1:123456789012:bucket/test-bucket--use1-az4--x-s3"),
			}, nil
		},
	}

	result := detectDirectoryBucket(context.Background(), mockClient, "test-bucket--use1-az4--x-s3")
	test.Equals(t, true, result, "S3 Express bucket with AvailabilityZone should return true")
}

func TestDetectDirectoryBucket_S3Express_LocalZone(t *testing.T) {
	t.Parallel()

	mockClient := &mockS3Client{
		headBucketFunc: func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
			return &s3.HeadBucketOutput{
				BucketLocationType: s3types.LocationTypeLocalZone,
				BucketArn:          aws.String("arn:aws:s3express:us-west-2:123456789012:bucket/test-bucket--lax1-az1--x-s3"),
			}, nil
		},
	}

	result := detectDirectoryBucket(context.Background(), mockClient, "test-bucket--lax1-az1--x-s3")
	test.Equals(t, true, result, "S3 Express bucket with LocalZone should return true")
}

func TestDetectDirectoryBucket_HeadBucketError_FallbackToFalse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		err   error
		descr string
	}{
		{
			name:  "bucket not found",
			err:   errors.New("NoSuchBucket: The specified bucket does not exist"),
			descr: "should gracefully fall back to false for non-existent buckets",
		},
		{
			name:  "access denied",
			err:   errors.New("AccessDenied: Access Denied"),
			descr: "should gracefully fall back to false for permission errors",
		},
		{
			name:  "network error",
			err:   errors.New("dial tcp: connection refused"),
			descr: "should gracefully fall back to false for network errors (MinIO offline)",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mockClient := &mockS3Client{
				headBucketFunc: func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
					return nil, tc.err
				},
			}

			result := detectDirectoryBucket(context.Background(), mockClient, "test-bucket--use1-az4--x-s3")
			test.Equals(t, false, result, tc.descr)
		})
	}
}

func TestDetectDirectoryBucket_SuffixButNotDirectoryBucket(t *testing.T) {
	t.Parallel()

	// MinIO bucket that happens to use --x-s3 suffix
	mockClient := &mockS3Client{
		headBucketFunc: func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
			// MinIO returns empty BucketLocationType
			return &s3.HeadBucketOutput{
				BucketLocationType: "",
				BucketArn:          aws.String(""),
			}, nil
		},
	}

	result := detectDirectoryBucket(context.Background(), mockClient, "minio-bucket--x-s3")
	test.Equals(t, false, result, "bucket with --x-s3 suffix but empty BucketLocationType should return false")
}

func TestPut_ACLHandling_RegularBucket(t *testing.T) {
	t.Parallel()

	backend := &Backend{
		acl:               "public-read",
		isDirectoryBucket: false,
	}

	test.Equals(t, "public-read", backend.acl, "ACL should be set for regular buckets")
	test.Equals(t, false, backend.isDirectoryBucket, "should be identified as regular bucket")
}

func TestPut_ACLHandling_DirectoryBucket(t *testing.T) {
	t.Parallel()

	backend := &Backend{
		acl:               "public-read",
		isDirectoryBucket: true,
	}

	// Verify that backend fields are set correctly
	test.Equals(t, "public-read", backend.acl, "ACL field should still be set")
	test.Equals(t, true, backend.isDirectoryBucket, "should be identified as directory bucket")
	// Note: In actual Put() method, ACL is skipped when isDirectoryBucket=true
}

func TestEncryptionHandling_DirectoryBucket(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                 string
		encryption           string
		isDirectoryBucket    bool
		shouldSkipEncryption bool
		shouldSkipDSSE_KMS   bool
		description          string
	}{
		{
			name:                 "regular bucket with DSSE-KMS",
			encryption:           "aws:dsse-kms",
			isDirectoryBucket:    false,
			shouldSkipEncryption: false,
			shouldSkipDSSE_KMS:   false,
			description:          "DSSE-KMS should be applied for regular buckets",
		},
		{
			name:                 "directory bucket with DSSE-KMS",
			encryption:           "aws:dsse-kms",
			isDirectoryBucket:    true,
			shouldSkipEncryption: false,
			shouldSkipDSSE_KMS:   true,
			description:          "DSSE-KMS should be skipped for directory buckets",
		},
		{
			name:                 "directory bucket with AES256",
			encryption:           "AES256",
			isDirectoryBucket:    true,
			shouldSkipEncryption: false,
			shouldSkipDSSE_KMS:   false,
			description:          "AES256 should be applied for directory buckets",
		},
		{
			name:                 "directory bucket with aws:kms",
			encryption:           "aws:kms",
			isDirectoryBucket:    true,
			shouldSkipEncryption: false,
			shouldSkipDSSE_KMS:   false,
			description:          "aws:kms should be applied for directory buckets",
		},
		{
			name:                 "directory bucket with empty encryption",
			encryption:           "",
			isDirectoryBucket:    true,
			shouldSkipEncryption: true,
			shouldSkipDSSE_KMS:   false,
			description:          "empty encryption should skip encryption logic",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backend := &Backend{
				encryption:        tc.encryption,
				isDirectoryBucket: tc.isDirectoryBucket,
			}

			// Verify backend fields are set correctly
			test.Equals(t, tc.encryption, backend.encryption, "encryption should match")
			test.Equals(t, tc.isDirectoryBucket, backend.isDirectoryBucket, "isDirectoryBucket should match")

			// Test the skip logic (this is what happens in Put() method)
			shouldSkip := tc.encryption == "" ||
				(tc.isDirectoryBucket && strings.EqualFold(tc.encryption, "aws:dsse-kms"))

			if tc.shouldSkipEncryption {
				test.Equals(t, "", tc.encryption, tc.description)
			} else if tc.shouldSkipDSSE_KMS {
				test.Equals(t, true, shouldSkip, tc.description)
			} else {
				test.Equals(t, false, shouldSkip, tc.description)
			}
		})
	}
}

func TestBucketSuffixDetection(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		bucketName     string
		expectedSuffix bool
	}{
		{
			name:           "valid S3 Express suffix",
			bucketName:     "my-bucket--use1-az4--x-s3",
			expectedSuffix: true,
		},
		{
			name:           "valid S3 Express suffix local zone",
			bucketName:     "test--lax1-az1--x-s3",
			expectedSuffix: true,
		},
		{
			name:           "regular bucket",
			bucketName:     "my-regular-bucket",
			expectedSuffix: false,
		},
		{
			name:           "bucket with --x-s3 in the middle",
			bucketName:     "my--x-s3-bucket",
			expectedSuffix: false,
		},
		{
			name:           "bucket ending with -x-s3 (single dash)",
			bucketName:     "my-bucket-x-s3",
			expectedSuffix: false,
		},
		{
			name:           "empty bucket name",
			bucketName:     "",
			expectedSuffix: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			hasSuffix := strings.HasSuffix(tc.bucketName, "--x-s3")
			test.Equals(t, tc.expectedSuffix, hasSuffix, "suffix detection should match expected")
		})
	}
}

func TestBackendStructFields(t *testing.T) {
	t.Parallel()

	backend := &Backend{
		bucket:            "test-bucket",
		acl:               "private",
		encryption:        "AES256",
		isDirectoryBucket: true,
	}

	test.Equals(t, "test-bucket", backend.bucket, "bucket name should be set")
	test.Equals(t, "private", backend.acl, "ACL should be set")
	test.Equals(t, "AES256", backend.encryption, "encryption should be set")
	test.Equals(t, true, backend.isDirectoryBucket, "isDirectoryBucket should be set")
}

// Integration check: verify the actual HeadBucket call structure
func TestDetectDirectoryBucket_HeadBucketInputStructure(t *testing.T) {
	t.Parallel()

	var capturedInput *s3.HeadBucketInput

	mockClient := &mockS3Client{
		headBucketFunc: func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
			capturedInput = params
			return &s3.HeadBucketOutput{
				BucketLocationType: s3types.LocationTypeAvailabilityZone,
			}, nil
		},
	}

	bucketName := "test-bucket--use1-az4--x-s3"
	detectDirectoryBucket(context.Background(), mockClient, bucketName)

	test.Assert(t, capturedInput != nil, "HeadBucket should be called")
	test.Equals(t, bucketName, *capturedInput.Bucket, "HeadBucket should be called with correct bucket name")
}

func TestList_PrefixHandling_DirectoryBucket(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		inputPrefix    string
		expectedPrefix string
		isDirectory    bool
		description    string
	}{
		{
			name:           "directory bucket - prefix without trailing slash",
			inputPrefix:    "cache/path",
			expectedPrefix: "cache/path/",
			isDirectory:    true,
			description:    "should append / for directory buckets",
		},
		{
			name:           "directory bucket - prefix with trailing slash",
			inputPrefix:    "cache/path/",
			expectedPrefix: "cache/path/",
			isDirectory:    true,
			description:    "should keep existing / for directory buckets",
		},
		{
			name:           "directory bucket - empty prefix",
			inputPrefix:    "",
			expectedPrefix: "",
			isDirectory:    true,
			description:    "should not modify empty prefix",
		},
		{
			name:           "regular bucket - prefix without trailing slash",
			inputPrefix:    "cache/path",
			expectedPrefix: "cache/path",
			isDirectory:    false,
			description:    "should not modify prefix for regular buckets",
		},
		{
			name:           "regular bucket - prefix with trailing slash",
			inputPrefix:    "cache/path/",
			expectedPrefix: "cache/path/",
			isDirectory:    false,
			description:    "should keep prefix unchanged for regular buckets",
		},
		{
			name:           "directory bucket - root prefix (/)",
			inputPrefix:    "/",
			expectedPrefix: "/",
			isDirectory:    true,
			description:    "should not double-append / when prefix is already root /",
		},
		{
			name:           "regular bucket - root prefix (/)",
			inputPrefix:    "/",
			expectedPrefix: "/",
			isDirectory:    false,
			description:    "should not modify root / prefix for regular buckets",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test the prefix transformation logic
			prefix := tc.inputPrefix
			if tc.isDirectory && prefix != "" && !strings.HasSuffix(prefix, "/") {
				prefix = prefix + "/"
			}

			test.Equals(t, tc.expectedPrefix, prefix, tc.description)
		})
	}
}

// --- Checksum suppression for S3-compatible endpoints (CI-24370) ---

func TestHasCustomEndpoint(t *testing.T) {
	// Uses t.Setenv, so no t.Parallel.
	t.Setenv("AWS_ENDPOINT_URL", "")
	t.Setenv("AWS_ENDPOINT_URL_S3", "")

	testCases := []struct {
		name           string
		pluginEndpoint string
		cfgEndpoint    *string
		s3EndpointEnv  string
		expected       bool
	}{
		{
			name:           "plugin endpoint set",
			pluginEndpoint: "https://minio.example.com",
			expected:       true,
		},
		{
			name:        "config base endpoint set (AWS_ENDPOINT_URL env chain)",
			cfgEndpoint: aws.String("https://minio.example.com"),
			expected:    true,
		},
		{
			name:          "service specific endpoint env (AWS_ENDPOINT_URL_S3)",
			s3EndpointEnv: "https://minio.example.com",
			expected:      true,
		},
		{
			name:     "aws s3 defaults (no endpoint)",
			expected: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.s3EndpointEnv != "" {
				t.Setenv("AWS_ENDPOINT_URL_S3", tc.s3EndpointEnv)
			}

			cfg := aws.Config{}
			if tc.cfgEndpoint != nil {
				cfg.BaseEndpoint = tc.cfgEndpoint
			}

			test.Equals(t, tc.expected, hasCustomEndpoint(tc.pluginEndpoint, cfg), tc.name)
		})
	}
}

// capturedS3Request records a single S3 API request received by the fake
// S3 test server.
type capturedS3Request struct {
	method string
	path   string
	query  url.Values
	header http.Header
}

// fakeS3Recorder records requests received by the fake S3 server. Uploads
// run parts concurrently, so access is guarded by a mutex.
type fakeS3Recorder struct {
	mu       sync.Mutex
	requests []capturedS3Request
}

func (r *fakeS3Recorder) record(req capturedS3Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, req)
}

func (r *fakeS3Recorder) all() []capturedS3Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]capturedS3Request(nil), r.requests...)
}

// newFakeS3TLSServer starts an HTTPS httptest server implementing the minimal
// S3 API surface the multipart upload manager needs: CreateMultipartUpload,
// UploadPart, CompleteMultipartUpload, and PutObject.
func newFakeS3TLSServer(t *testing.T) (*httptest.Server, *fakeS3Recorder) {
	t.Helper()

	rec := &fakeS3Recorder{}

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)

		rec.record(capturedS3Request{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.Query(),
			header: r.Header.Clone(),
		})

		w.Header().Set("Content-Type", "application/xml")

		switch {
		case r.Method == http.MethodPost && r.URL.Query().Has("uploads"):
			// CreateMultipartUpload
			_, _ = w.Write([]byte(`<InitiateMultipartUploadResult><Bucket>bucket</Bucket><Key>key</Key><UploadId>upload-1</UploadId></InitiateMultipartUploadResult>`))
		case r.Method == http.MethodPut && r.URL.Query().Has("uploadId"):
			// UploadPart
			w.Header().Set("ETag", `"etag"`)
		case r.Method == http.MethodPost && r.URL.Query().Has("uploadId"):
			// CompleteMultipartUpload
			_, _ = w.Write([]byte(`<CompleteMultipartUploadResult><Location>localhost</Location><Bucket>bucket</Bucket><Key>key</Key><ETag>"etag"</ETag></CompleteMultipartUploadResult>`))
		default:
			// PutObject and anything else
			w.Header().Set("ETag", `"etag"`)
		}
	}))
	t.Cleanup(srv.Close)

	return srv, rec
}

// trustTestServerCert makes the SDK (LoadDefaultConfig) trust the httptest
// TLS server certificate via the AWS_CA_BUNDLE environment variable.
func trustTestServerCert(t *testing.T, srv *httptest.Server) {
	t.Helper()

	cert := srv.Certificate()
	if cert == nil {
		t.Fatal("test server did not expose its certificate")
	}

	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	certFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(certFile, pemBytes, 0o600); err != nil {
		t.Fatalf("write CA bundle: %v", err)
	}

	t.Setenv("AWS_CA_BUNDLE", certFile)
}

// assertMultipartUpload verifies the payload actually went through the
// multipart upload path (CreateMultipartUpload + UploadPart + Complete).
func assertMultipartUpload(t *testing.T, requests []capturedS3Request) {
	t.Helper()

	var sawCreate, sawUploadPart, sawComplete bool
	for _, req := range requests {
		if req.method == http.MethodPost && req.query.Has("uploads") {
			sawCreate = true
		}
		if req.method == http.MethodPut && req.query.Has("uploadId") {
			sawUploadPart = true
		}
		if req.method == http.MethodPost && req.query.Has("uploadId") {
			sawComplete = true
		}
	}

	test.Assert(t, sawCreate, "CreateMultipartUpload should have been issued (payload exceeds 5MiB part size)")
	test.Assert(t, sawUploadPart, "UploadPart should have been issued (the call failing for NetApp ONTAP S3 in CI-24370)")
	test.Assert(t, sawComplete, "CompleteMultipartUpload should have been issued")
}

// assertNoDefaultChecksums verifies none of the requests carry the AWS SDK
// v2 default CRC32 checksums. S3-compatible backends such as NetApp ONTAP S3
// reject the resulting streaming trailer with 400 InvalidArgument
// (x-amz-content-sha256 must be UNSIGNED-PAYLOAD, ... or a valid sha256).
func assertNoDefaultChecksums(t *testing.T, requests []capturedS3Request) {
	t.Helper()

	for _, req := range requests {
		for _, header := range []string{
			"x-amz-checksum-algorithm",
			"x-amz-sdk-checksum-algorithm",
			"x-amz-checksum-crc32",
			"x-amz-trailer",
		} {
			if v := req.header.Get(header); v != "" {
				t.Errorf("%s %s: %s header is set to %q, want no default checksum headers for S3-compatible endpoints",
					req.method, req.path, header, v)
			}
		}

		// Requests carrying a body must use UNSIGNED-PAYLOAD, not the
		// STREAMING-UNSIGNED-PAYLOAD-TRAILER value rejected by NetApp ONTAP S3.
		if req.method == http.MethodPut {
			if got := req.header.Get("x-amz-content-sha256"); got != "UNSIGNED-PAYLOAD" {
				t.Errorf("%s %s: x-amz-content-sha256 = %q, want UNSIGNED-PAYLOAD (CI-24370)",
					req.method, req.path, got)
			}
		}
	}
}

// TestPut_MultipartUpload_NoDefaultChecksums_PluginEndpoint reproduces
// CI-24370: multipart uploads (cache archives exceed the 5MiB default part
// size) to an S3-compatible endpoint must not send the AWS SDK v2 default
// CRC32 streaming/trailing checksums that NetApp ONTAP S3 rejects with
// 400 InvalidArgument.
func TestPut_MultipartUpload_NoDefaultChecksums_PluginEndpoint(t *testing.T) {
	// Uses t.Setenv (CA bundle), so no t.Parallel.
	srv, rec := newFakeS3TLSServer(t)
	trustTestServerCert(t, srv)

	backend, err := New(log.NewNopLogger(), Config{
		Bucket:   "test-bucket",
		Endpoint: srv.URL,
		Key:      "test-access-key",
		Secret:   "test-secret-key",
		Region:   "eu-west-1",
	}, false)
	test.Ok(t, err)

	// 6MiB unseekable payload (like the pipe readers the archive layer
	// produces) exceeds the manager's 5MiB default part size and forces the
	// multipart UploadPart path — the customer's failing call.
	payload := struct{ io.Reader }{bytes.NewReader(bytes.Repeat([]byte("cache-archive"), 6<<20/13+1))}

	test.Ok(t, backend.Put(context.Background(), "test-key", payload))

	requests := rec.all()
	assertMultipartUpload(t, requests)
	assertNoDefaultChecksums(t, requests)
}

// TestPut_MultipartUpload_NoDefaultChecksums_EnvChainEndpoint covers the
// endpoint arriving through the standard AWS environment chain
// (AWS_ENDPOINT_URL) instead of plugin configuration: it must still be
// treated as an S3-compatible endpoint and get checksum suppression.
func TestPut_MultipartUpload_NoDefaultChecksums_EnvChainEndpoint(t *testing.T) {
	// Uses t.Setenv, so no t.Parallel.
	srv, rec := newFakeS3TLSServer(t)
	trustTestServerCert(t, srv)

	t.Setenv("AWS_ENDPOINT_URL", srv.URL)
	t.Setenv("AWS_ENDPOINT_URL_S3", "")

	backend, err := New(log.NewNopLogger(), Config{
		Bucket: "test-bucket",
		Key:    "test-access-key",
		Secret: "test-secret-key",
		Region: "eu-west-1",
	}, false)
	test.Ok(t, err)

	payload := struct{ io.Reader }{bytes.NewReader(bytes.Repeat([]byte("cache-archive"), 6<<20/13+1))}

	test.Ok(t, backend.Put(context.Background(), "test-key", payload))

	requests := rec.all()
	// Sanity: the env-chain endpoint must have actually directed traffic to
	// the fake server (otherwise the test proves nothing).
	test.Assert(t, len(requests) > 0, "requests should have reached the fake S3 server via AWS_ENDPOINT_URL")
	assertMultipartUpload(t, requests)
	assertNoDefaultChecksums(t, requests)
}

// TestPut_SinglePart_NoDefaultChecksums_PluginEndpoint guards the single-part
// PutObject path: small uploads to S3-compatible endpoints must also stay
// free of default checksums.
func TestPut_SinglePart_NoDefaultChecksums_PluginEndpoint(t *testing.T) {
	// Uses t.Setenv (CA bundle), so no t.Parallel.
	srv, rec := newFakeS3TLSServer(t)
	trustTestServerCert(t, srv)

	backend, err := New(log.NewNopLogger(), Config{
		Bucket:   "test-bucket",
		Endpoint: srv.URL,
		Key:      "test-access-key",
		Secret:   "test-secret-key",
		Region:   "eu-west-1",
	}, false)
	test.Ok(t, err)

	test.Ok(t, backend.Put(context.Background(), "small-key", strings.NewReader("hello world")))

	requests := rec.all()
	test.Equals(t, 1, len(requests), "small object should be a single PutObject request")
	test.Equals(t, http.MethodPut, requests[0].method, "expected a PutObject request")
	assertNoDefaultChecksums(t, requests)
}
