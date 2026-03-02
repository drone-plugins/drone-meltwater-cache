//go:build integration
// +build integration

package s3

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	backend, cleanUp := setup(t, Config{
		ACL:       acl,
		Bucket:    "s3-round-trip",
		Endpoint:  endpoint,
		Key:       accessKey,
		PathStyle: true, // Should be true for minio and false for AWS.
		Region:    defaultRegion,
		Secret:    secretAccessKey,
	})
	t.Cleanup(cleanUp)
	roundTrip(t, backend)
}

func TestRoundTripWithAssumeRoleAndExternalID(t *testing.T) {
	t.Parallel()

	// Log the credentials being used for the test (without exposing secrets)
	logrus.WithFields(logrus.Fields{
		"RoleARN": "arn:aws:iam::account-id:role/TestRole",
	}).Info("Setting up AssumeRole test")

	// Setup backend using the assumed role credentials
	backend, cleanUp := setup(t, Config{
		ACL:                   acl,
		Bucket:                "s3-round-trip-with-role",
		Endpoint:              endpoint,
		StsEndpoint:           endpoint,
		PathStyle:             true,
		Key:                   accessKey,
		Secret:                secretAccessKey,
		Region:                defaultRegion,
		AssumeRoleARN:         "arn:aws:iam::account-id:role/TestRole",
		AssumeRoleSessionName: "drone-cache",
		ExternalID:            "example-external-id",
		UserRoleExternalID:    "example-external-id",
	})

	// Cleanup after the test
	t.Cleanup(cleanUp)

	// Perform the round-trip test
	roundTrip(t, backend)
}

func roundTrip(t *testing.T, backend *Backend) {
	content := "Hello world4"

	// Test Put
	test.Ok(t, backend.Put(context.TODO(), "test.t", strings.NewReader(content)))

	// Test Get
	var buf bytes.Buffer
	test.Ok(t, backend.Get(context.TODO(), "test.t", &buf))

	b, err := io.ReadAll(&buf)
	test.Ok(t, err)

	test.Equals(t, []byte(content), b)

	exists, err := backend.Exists(context.TODO(), "test.t")
	test.Ok(t, err)

	test.Equals(t, true, exists)

	entries, err := backend.List(context.TODO(), "")
	test.Ok(t, err)
	test.Equals(t, 1, len(entries))
}

// Helpers

func setup(t *testing.T, cfg Config) (*Backend, func()) {
	client := newClient(cfg)
	ctx := context.Background()

	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(cfg.Bucket),
	})
	test.Ok(t, err)

	b, err := New(
		log.NewNopLogger(),
		cfg,
		false,
	)
	test.Ok(t, err)

	return b, func() {
		_, _ = client.DeleteBucket(ctx, &s3.DeleteBucketInput{
			Bucket: aws.String(cfg.Bucket),
		})
	}
}

func newClient(cfg Config) *s3.Client {
	ctx := context.Background()

	var optFns []func(*config.LoadOptions) error
	optFns = append(optFns, config.WithRegion(defaultRegion))

	if cfg.Key != "" && cfg.Secret != "" {
		optFns = append(optFns, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.Key, cfg.Secret, ""),
		))
	}

	// Handle custom endpoint
	if endpoint != "" {
		customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:               endpoint,
				HostnameImmutable: true,
				SigningRegion:     defaultRegion,
			}, nil
		})
		optFns = append(optFns, config.WithEndpointResolverWithOptions(customResolver))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to load AWS config")
	}

	logrus.WithFields(logrus.Fields{
		"Region":   defaultRegion,
		"Endpoint": endpoint,
	}).Info("Creating new S3 client")

	return s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})
}

func getEnv(key, defaultVal string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		return defaultVal
	}

	return value
}
