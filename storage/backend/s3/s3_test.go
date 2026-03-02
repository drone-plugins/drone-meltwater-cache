//go:build integration
// +build integration

package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
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
		PathStyle: true,
		Region:    defaultRegion,
		Secret:    secretAccessKey,
	})
	t.Cleanup(cleanUp)
	roundTrip(t, backend)
}

func TestRoundTripWithAssumeRoleAndExternalID(t *testing.T) {
	t.Parallel()

	logrus.WithFields(logrus.Fields{
		"RoleARN": "arn:aws:iam::account-id:role/TestRole",
	}).Info("Setting up AssumeRole test")

	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(defaultRegion),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, secretAccessKey, ""),
		),
	)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	stsSvc := sts.NewFromConfig(cfg)
	provider := stscreds.NewAssumeRoleProvider(stsSvc, "arn:aws:iam::account-id:role/TestRole", func(o *stscreds.AssumeRoleOptions) {
		o.ExternalID = aws.String("example-external-id")
		logrus.WithField("externalID", "example-external-id").Info("Using external ID for assume role")
	})

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
		Credentials:           aws.NewCredentialsCache(provider),
	})

	t.Cleanup(cleanUp)
	roundTrip(t, backend)
}

func roundTrip(t *testing.T, backend *Backend) {
	content := "Hello world4"

	test.Ok(t, backend.Put(context.TODO(), "test.t", strings.NewReader(content)))

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

func setup(t *testing.T, config Config) (*Backend, func()) {
	ctx := context.Background()
	client := newClient(ctx, config)

	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(config.Bucket),
	})
	test.Ok(t, err)

	b, err := New(
		log.NewNopLogger(),
		config,
		false,
	)
	test.Ok(t, err)

	return b, func() {
		_, _ = client.DeleteBucket(ctx, &s3.DeleteBucketInput{
			Bucket: aws.String(config.Bucket),
		})
	}
}

func newClient(ctx context.Context, config Config) *s3.Client {
	optFns := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(defaultRegion),
	}

	if config.Key != "" && config.Secret != "" {
		optFns = append(optFns, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(config.Key, config.Secret, ""),
		))
	}

	logrus.WithFields(logrus.Fields{
		"Region":    defaultRegion,
		"Endpoint":  endpoint,
		"AccessKey": config.Key,
	}).Info("Creating new S3 client")

	cfg, err := awsconfig.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}

	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
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
