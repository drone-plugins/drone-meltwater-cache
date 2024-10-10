//go:build integration
// +build integration

package s3

import (
	"bytes"
	"context"
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
	defaultEndpoint            = "http://127.0.0.1:9000"
	defaultAccessKey           = "AKIAIOSFODNN7EXAMPLE"
	defaultSecretAccessKey     = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	defaultRegion              = "eu-west-1"
	defaultACL                 = "private"
	defaultUserAccessKey       = "foo"
	defaultUserSecretAccessKey = "barbarbar"
)

func TestBasicS3Operations(t *testing.T) {
	t.Skip()
	t.Parallel()

	bucketName := "s3-round-trip"
	
	config := Config{
		Bucket:     bucketName,
		Endpoint:   defaultEndpoint,
		Region:     defaultRegion,
		ACL:        defaultACL,
		PathStyle:  true,
		Key:        defaultAccessKey,
		Secret:     defaultSecretAccessKey,
	}

	backend, cleanup := setupTest(t, config)
	defer cleanup()

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

func TestS3WithAssumeRole(t *testing.T) {
	t.Skip("Skipping assume role test in local environment")
	
	bucketName := "s3-round-trip-with-role"
	
	config := Config{
		Bucket:                bucketName,
		Endpoint:              defaultEndpoint,
		Region:                defaultRegion,
		ACL:                   defaultACL,
		PathStyle:             true,
		AssumeRoleARN:         "arn:aws:iam::account-id:role/TestRole",
		AssumeRoleSessionName: "test-session",
		ExternalID:            "test-external-id",
		Key:                   defaultUserAccessKey,
		Secret:                defaultUserSecretAccessKey,
	}

	backend, cleanup := setupTest(t, config)
	defer cleanup()

	content := "Hello World Test Content with Assume Role"
	key := "test-file-assume-role.txt"

	err := backend.Put(context.Background(), key, strings.NewReader(content))
	test.Ok(t, err)

	var buf bytes.Buffer
	err = backend.Get(context.Background(), key, &buf)
	test.Ok(t, err)
	test.Equals(t, content, buf.String())
}

func setupTest(t *testing.T, config Config) (*Backend, func()) {
	s3Client := createS3Client(config)
	
	_, err := s3Client.CreateBucket(&s3.CreateBucketInput{
		Bucket: aws.String(config.Bucket),
	})
	test.Ok(t, err)

	err = s3Client.WaitUntilBucketExists(&s3.HeadBucketInput{
		Bucket: aws.String(config.Bucket),
	})
	test.Ok(t, err)

	backend, err := New(log.NewNopLogger(), config, true)
	test.Ok(t, err)

	cleanup := func() {
		err := deleteAllObjects(s3Client, config.Bucket)
		if err != nil {
			t.Logf("Failed to delete objects: %v", err)
		}

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
		Credentials:      aws.NewStaticCredentials(config.Key, config.Secret, ""),
	}

	sess, err := session.NewSession(awsConfig)
	if err != nil {
		panic(err)
	}
	return s3.New(sess)
}

func deleteAllObjects(s3Client *s3.S3, bucket string) error {
	listOutput, err := s3Client.ListObjects(&s3.ListObjectsInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return err
	}

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