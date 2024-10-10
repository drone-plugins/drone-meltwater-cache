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
		AssumeRoleSessionName: "drone-cache",
		ExternalID:            "example-external-id",
		UserRoleExternalID:    "example-external-id",
	})
	t.Cleanup(cleanUp)
	roundTrip(t, backend)
}

func roundTrip(t *testing.T, backend *Backend) {
	content := "Hello world4"

	// Test Put
	test.Ok(t, backend.Put(context.TODO(), "test.t", strings.NewReader(content)))

	// Test Get
	var buf bytes.Buffer
	test.Ok(t, backend.Get(context.TODO(), "test.t", &buf))

	b, err := ioutil.ReadAll(&buf)
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
	client := newClient(config)

	_, err := client.CreateBucketWithContext(context.Background(), &s3.CreateBucketInput{
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
		_, err = client.DeleteBucket(&s3.DeleteBucketInput{
			Bucket: aws.String(config.Bucket),
		})
	}
}

func newClient(config Config) *s3.S3 {
	var creds *credentials.Credentials
	if config.Key != "" && config.Secret != "" {
		creds = credentials.NewStaticCredentials(config.Key, config.Secret, "")
	} else {
		creds = credentials.NewEnvCredentials()
		logrus.Info("Using environment-based credentials for S3 client")
	}

	conf := &aws.Config{
		Region:           aws.String(defaultRegion),
		Endpoint:         aws.String(endpoint),
		DisableSSL:       aws.Bool(strings.HasPrefix(endpoint, "http://")),
		S3ForcePathStyle: aws.Bool(true),
		Credentials:      creds,
		CredentialsChainVerboseErrors: aws.Bool(true),
	}

	logrus.WithFields(logrus.Fields{
		"Region":    defaultRegion,
		"Endpoint":  endpoint,
		"AccessKey": config.Key,
	}).Info("Creating new S3 client")

	return s3.New(session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	})), conf)
}

func getEnv(key, defaultVal string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		return defaultVal
	}

	return value
}