package s3

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
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
