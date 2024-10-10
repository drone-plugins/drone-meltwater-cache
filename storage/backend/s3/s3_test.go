//go:build integration
// +build integration

package s3

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/go-kit/kit/log"
	"github.com/meltwater/drone-cache/test"
)

const (
	defaultEndpoint       = "http://localhost:9000"
	defaultRegion         = "us-east-1"
	defaultACL            = "private"
	defaultBucketPrefix   = "test-bucket"
)

// TestBasicS3Operations tests basic S3 operations without explicit credentials
func TestBasicS3Operations(t *testing.T) {
	t.Parallel()

	bucketName := defaultBucketPrefix + "-basic-" + time.Now().Format("20060102150405")
	
	config := Config{
		Bucket:     bucketName,
		Endpoint:   defaultEndpoint,
		Region:     defaultRegion,
		ACL:        defaultACL,
		PathStyle:  true,
	}

	// Setup backend and cleanup
	backend, cleanup := setupTest(t, config)
	defer cleanup()

	// Test file content
	content := "Hello World Test Content"
	key := "test-file.txt"

	// Test Put operation
	err := backend.Put(context.Background(), key, strings.NewReader(content))
	test.Ok(t, err)

	// Test Exists operation
	exists, err := backend.Exists(context.Background(), key)
	test.Ok(t, err)
	test.Equals(t, true, exists)

	// Test Get operation
	var buf bytes.Buffer
	err = backend.Get(context.Background(), key, &buf)
	test.Ok(t, err)
	test.Equals(t, content, buf.String())

	// Test List operation
	entries, err := backend.List(context.Background(), "")
	test.Ok(t, err)
	test.Equals(t, 1, len(entries))
	test.Equals(t, key, entries[0].Path)
}

// TestS3WithAssumeRole tests S3 operations with assume role
func TestS3WithAssumeRole(t *testing.T) {
	t.Parallel()

	bucketName := defaultBucketPrefix + "-assume-role-" + time.Now().Format("20060102150405")
	
	config := Config{
		Bucket:                bucketName,
		Endpoint:              defaultEndpoint,
		Region:                defaultRegion,
		ACL:                   defaultACL,
		PathStyle:             true,
		AssumeRoleARN:         "arn:aws:iam::123456789012:role/test-role",
		AssumeRoleSessionName: "test-session",
		ExternalID:            "test-external-id",
	}

	// Setup backend and cleanup
	backend, cleanup := setupTest(t, config)
	defer cleanup()

	// Test file content
	content := "Hello World Test Content with Assume Role"
	key := "test-file-assume-role.txt"

	// Test Put operation
	err := backend.Put(context.Background(), key, strings.NewReader(content))
	test.Ok(t, err)

	// Test Get operation
	var buf bytes.Buffer
	err = backend.Get(context.Background(), key, &buf)
	test.Ok(t, err)
	test.Equals(t, content, buf.String())
}

// TestS3WithOIDC tests S3 operations with OIDC token
// func TestS3WithOIDC(t *testing.T) {
// 	t.Parallel()

// 	bucketName := defaultBucketPrefix + "-oidc-" + time.Now().Format("20060102150405")
	
// 	config := Config{
// 		Bucket:                bucketName,
// 		Endpoint:              defaultEndpoint,
// 		Region:                defaultRegion,
// 		ACL:                   defaultACL,
// 		PathStyle:             true,
// 		AssumeRoleARN:         "arn:aws:iam::123456789012:role/test-role",
// 		AssumeRoleSessionName: "test-session",
// 		OIDCTokenID:           "test-oidc-token",
// 	}

// 	// Setup backend and cleanup
// 	backend, cleanup := setupTest(t, config)
// 	defer cleanup()

// 	// Test file content
// 	content := "Hello World Test Content with OIDC"
// 	key := "test-file-oidc.txt"

// 	// Test Put operation
// 	err := backend.Put(context.Background(), key, strings.NewReader(content))
// 	test.Ok(t, err)

// 	// Test Get operation
// 	var buf bytes.Buffer
// 	err = backend.Get(context.Background(), key, &buf)
// 	test.Ok(t, err)
// 	test.Equals(t, content, buf.String())
// }

// // TestS3WithUserRole tests S3 operations with user role
// func TestS3WithUserRole(t *testing.T) {
// 	t.Parallel()

// 	bucketName := defaultBucketPrefix + "-user-role-" + time.Now().Format("20060102150405")
	
// 	config := Config{
// 		Bucket:              bucketName,
// 		Endpoint:            defaultEndpoint,
// 		Region:              defaultRegion,
// 		ACL:                 defaultACL,
// 		PathStyle:           true,
// 		UserRoleArn:         "arn:aws:iam::123456789012:role/test-user-role",
// 		UserRoleExternalID:  "test-user-external-id",
// 	}

// 	// Setup backend and cleanup
// 	backend, cleanup := setupTest(t, config)
// 	defer cleanup()

// 	// Test file content
// 	content := "Hello World Test Content with User Role"
// 	key := "test-file-user-role.txt"

// 	// Test Put operation
// 	err := backend.Put(context.Background(), key, strings.NewReader(content))
// 	test.Ok(t, err)

// 	// Test Get operation
// 	var buf bytes.Buffer
// 	err = backend.Get(context.Background(), key, &buf)
// 	test.Ok(t, err)
// 	test.Equals(t, content, buf.String())
// }

// Helper functions
func setupTest(t *testing.T, config Config) (*Backend, func()) {
	// Create S3 client
	s3Client := createS3Client(config)
	
	// Create test bucket
	_, err := s3Client.CreateBucket(&s3.CreateBucketInput{
		Bucket: aws.String(config.Bucket),
	})
	test.Ok(t, err)

	// Wait for bucket to be created
	err = s3Client.WaitUntilBucketExists(&s3.HeadBucketInput{
		Bucket: aws.String(config.Bucket),
	})
	test.Ok(t, err)

	// Create backend
	backend, err := New(log.NewNopLogger(), config, true)
	test.Ok(t, err)

	// Return cleanup function
	cleanup := func() {
		// Delete all objects in bucket
		err := deleteAllObjects(s3Client, config.Bucket)
		if err != nil {
			t.Logf("Failed to delete objects: %v", err)
		}

		// Delete bucket
		_, err = s3Client.DeleteBucket(&s3.DeleteBucketInput{
			Bucket: aws.String(config.Bucket),
		})
		if err != nil {
			t.Logf("Failed to delete bucket: %v", err)
		}
	}

	return backend, cleanup
}

func createS3Client(config Config) *s3.S3 {
	awsConfig := &aws.Config{
		Endpoint:         aws.String(config.Endpoint),
		Region:           aws.String(config.Region),
		DisableSSL:       aws.Bool(strings.HasPrefix(config.Endpoint, "http://")),
		S3ForcePathStyle: aws.Bool(config.PathStyle),
	}

	sess, err := session.NewSession(awsConfig)
	if err != nil {
		panic(err)
	}
	return s3.New(sess)
}

func deleteAllObjects(s3Client *s3.S3, bucket string) error {
	// List all objects in bucket
	listOutput, err := s3Client.ListObjects(&s3.ListObjectsInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return err
	}

	// Delete each object
	for _, object := range listOutput.Contents {
		_, err := s3Client.DeleteObject(&s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    object.Key,
		})
		if err != nil {
			return err
		}
	}

	return nil
}