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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/go-kit/kit/log"
	"github.com/sirupsen/logrus"

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

var (
	endpoint            = getEnv("TEST_S3_ENDPOINT", defaultEndpoint)
	accessKey           = getEnv("TEST_S3_ACCESS_KEY", defaultAccessKey)
	secretAccessKey     = getEnv("TEST_S3_SECRET_KEY", defaultSecretAccessKey)
	acl                 = getEnv("TEST_S3_ACL", defaultACL)
	userAccessKey       = getEnv("TEST_USER_S3_ACCESS_KEY", defaultUserAccessKey)
	userSecretAccessKey = getEnv("TEST_USER_S3_SECRET_KEY", defaultUserSecretAccessKey)
)

func TestBasicRoundTrip(t *testing.T) {
	t.Parallel()

	// Set up a logger for the test
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	logger.WithFields(logrus.Fields{
		"AccessKey": accessKey,
		"Endpoint":  endpoint,
		"Region":    defaultRegion,
	}).Info("Setting up basic S3 round trip test")

	// Create a test configuration
	config := Config{
		ACL:       acl,
		Bucket:    "s3-basic-round-trip",
		Endpoint:  endpoint,
		Key:       accessKey,
		Secret:    secretAccessKey,
		PathStyle: true, // Should be true for minio and false for AWS.
		Region:    defaultRegion,
	}

	// Create a new backend
	backend, err := New(log.NewLogfmtLogger(os.Stdout), config, true)
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}

	// Set up the S3 bucket
	cleanup := setupTestBucket(t, config)
	t.Cleanup(cleanup)

	// Perform the round trip test
	testRoundTrip(t, backend)

	logger.Info("Basic S3 round trip test completed successfully")
}

func TestRoundTripWithAssumeRole(t *testing.T) {
	t.Parallel()

	// Log the credentials being used for the test
	logrus.WithFields(logrus.Fields{
		"AccessKey": userAccessKey,
		"SecretKey": userSecretAccessKey,
		"RoleARN":   "arn:aws:iam::account-id:role/TestRole",
	}).Info("Setting up AssumeRole test")

	backend, cleanUp := setup(t, Config{
		ACL:                   acl,
		Bucket:                "s3-round-trip-with-role",
		Endpoint:              endpoint,
		StsEndpoint:           endpoint,
		Key:                   userAccessKey,
		PathStyle:             true,
		Region:                defaultRegion,
		Secret:                userSecretAccessKey,
		AssumeRoleARN:         "arn:aws:iam::account-id:role/TestRole",
		UserRoleArn:           "arn:aws:iam::account-id:role/TestRole",
		AssumeRoleSessionName: "drone-cache",
		ExternalID:            "example-external-id",
		UserRoleExternalID:    "example-external-id",
	})
	t.Cleanup(cleanUp)
	roundTrip(t, backend)
}

func setupTestBucket(t *testing.T, config Config) func() {
	// Create an S3 client
	sess, err := session.NewSession(&aws.Config{
		Region:           aws.String(config.Region),
		Endpoint:         aws.String(config.Endpoint),
		Credentials:      credentials.NewStaticCredentials(config.Key, config.Secret, ""),
		S3ForcePathStyle: aws.Bool(config.PathStyle),
	})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	client := s3.New(sess)

	// Create the bucket
	_, err = client.CreateBucket(&s3.CreateBucketInput{
		Bucket: aws.String(config.Bucket),
	})
	if err != nil {
		t.Fatalf("Failed to create bucket: %v", err)
	}

	return func() {
		// Delete all objects in the bucket
		err := deleteAllObjects(client, config.Bucket)
		if err != nil {
			t.Logf("Failed to delete objects in bucket: %v", err)
		}

		// Delete the bucket
		_, err = client.DeleteBucket(&s3.DeleteBucketInput{
			Bucket: aws.String(config.Bucket),
		})
		if err != nil {
			t.Logf("Failed to delete bucket: %v", err)
		}
	}
}

func deleteAllObjects(client *s3.S3, bucket string) error {
	// List all objects in the bucket
	listResp, err := client.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return fmt.Errorf("failed to list objects: %w", err)
	}

	// Delete each object
	for _, item := range listResp.Contents {
		_, err := client.DeleteObject(&s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    item.Key,
		})
		if err != nil {
			return fmt.Errorf("failed to delete object %s: %w", *item.Key, err)
		}
	}

	return nil
}

func testRoundTrip(t *testing.T, backend *Backend) {
	content := "Hello, S3 world!"
	key := "test-file.txt"

	// Test Put
	err := backend.Put(context.TODO(), key, strings.NewReader(content))
	if err != nil {
		t.Fatalf("Failed to put object: %v", err)
	}

	// Test Get
	var buf bytes.Buffer
	err = backend.Get(context.TODO(), key, &buf)
	if err != nil {
		t.Fatalf("Failed to get object: %v", err)
	}

	if buf.String() != content {
		t.Fatalf("Retrieved content does not match. Expected: %s, Got: %s", content, buf.String())
	}

	// Test Exists
	exists, err := backend.Exists(context.TODO(), key)
	if err != nil {
		t.Fatalf("Failed to check if object exists: %v", err)
	}
	if !exists {
		t.Fatalf("Object should exist, but Exists returned false")
	}

	// Test List
	entries, err := backend.List(context.TODO(), "")
	if err != nil {
		t.Fatalf("Failed to list objects: %v", err)
	}
	if len(entries) != 1 || entries[0] != key {
		t.Fatalf("Unexpected list result. Expected: [%s], Got: %v", key, entries)
	}
}

func newClient(config Config) *s3.S3 {
    conf := &aws.Config{
        Region:           aws.String(defaultRegion),
        Endpoint:         aws.String(endpoint),
        DisableSSL:       aws.Bool(strings.HasPrefix(endpoint, "http://")),
        S3ForcePathStyle: aws.Bool(true),
    }

    // Create initial session
    sess, err := session.NewSession(conf)
    if err != nil {
        logrus.WithError(err).Fatal("Could not create initial session")
    }

    // Setup credentials
    if config.Key != "" && config.Secret != "" {
        conf.Credentials = credentials.NewStaticCredentials(config.Key, config.Secret, "")
    } else {
        conf.Credentials = credentials.NewEnvCredentials()
    }

    // Create new session with updated configuration
    sess, err = session.NewSession(conf)
    if err != nil {
        logrus.WithError(err).Fatal("Could not create session with credentials")
    }

    logrus.WithFields(logrus.Fields{
        "Region":    defaultRegion,
        "Endpoint":  endpoint,
        "AccessKey": config.Key,
    }).Info("Creating new S3 client")

    return s3.New(sess, conf)
}

func getEnv(key, defaultVal string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		return defaultVal
	}

	return value
}
