package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	s3manager "github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	"github.com/go-kit/log"
	"github.com/sirupsen/logrus"

	"github.com/meltwater/drone-cache/internal"
	"github.com/meltwater/drone-cache/storage/common"
)

// Backend implements storage.Backend for AWS S3.
type Backend struct {
	logger log.Logger

	bucket            string
	acl               string
	encryption        string
	client            *s3.Client
	uploader          *s3manager.Uploader
	isDirectoryBucket bool
}

// New creates a new S3 backend with lazy-loaded credentials.
func New(l log.Logger, c Config, debug bool) (*Backend, error) {
	ctx := context.Background()
	endpoint := normalizeEndpoint(c.Endpoint)

	optFns := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(c.Region),
	}

	if c.Key != "" && c.Secret != "" {
		if c.SessionToken != "" {
			logrus.Info("Using static credentials with session token (temporary credentials)")
		} else {
			logrus.Info("Using static credentials (access key and secret key)")
		}
		optFns = append(optFns, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(c.Key, c.Secret, c.SessionToken),
		))
	} else if c.Credentials != nil {
		logrus.Info("Using provided credentials")
		optFns = append(optFns, awsconfig.WithCredentialsProvider(c.Credentials))
	} else if c.AssumeRoleARN != "" {
		if c.OIDCTokenID != "" {
			logrus.Info("Attempting to assume role with OIDC")
			creds, err := assumeRoleWithWebIdentity(ctx, c.AssumeRoleARN, c.AssumeRoleSessionName, c.OIDCTokenID, c.Region)
			if err != nil {
				logrus.WithError(err).Error("Failed to assume role with OIDC")
				return nil, err
			}
			optFns = append(optFns, awsconfig.WithCredentialsProvider(creds))
			logrus.Info("Successfully assumed role with OIDC")
		} else {
			creds := assumeRole(ctx, c.AssumeRoleARN, c.AssumeRoleSessionName, c.ExternalID, c.Region)
			optFns = append(optFns, awsconfig.WithCredentialsProvider(creds))
		}
	} else {
		logrus.Warn("No AWS credentials provided or role assumed; using default machine credentials for AWS requests")
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		logrus.WithError(err).Error("Could not load AWS config")
		return nil, fmt.Errorf("AWS config loading failed: %w", err)
	}

	s3Opts := []func(*s3.Options){}
	if endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = c.PathStyle
			// S3-compatible services (MinIO, Spaces, B2, etc.) may not support the
			// CRC32 checksums that SDK v2 sends by default. Pipe-based uploads are
			// also unseekable and break trailing checksums over plain HTTP.
			o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		})
	} else if c.PathStyle {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(cfg, s3Opts...)

	if len(c.UserRoleArn) > 0 {
		logrus.WithFields(logrus.Fields{
			"UserRoleArn":        c.UserRoleArn,
			"UserRoleExternalID": c.UserRoleExternalID,
		}).Info("Setting up credentials with UserRoleArn")

		stsSvc := sts.NewFromConfig(cfg)
		provider := stscreds.NewAssumeRoleProvider(stsSvc, c.UserRoleArn, func(o *stscreds.AssumeRoleOptions) {
			if c.UserRoleExternalID != "" {
				logrus.WithField("ExternalID", c.UserRoleExternalID).Info("Setting up creds with UserRoleExternalID")
				o.ExternalID = aws.String(c.UserRoleExternalID)
			}
		})

		cfg.Credentials = aws.NewCredentialsCache(provider)
		client = s3.NewFromConfig(cfg, s3Opts...)
		logrus.Info("Created S3 client with assumed user role")
	}

	logrus.WithFields(logrus.Fields{
		"Client": client,
	}).Info("New Client set here.")

	backend := &Backend{
		logger:            l,
		bucket:            c.Bucket,
		encryption:        c.Encryption,
		client:            client,
		uploader:          s3manager.NewUploader(client),
		isDirectoryBucket: detectDirectoryBucket(ctx, client, c.Bucket),
	}

	logrus.WithFields(logrus.Fields{
		"bucket":            c.Bucket,
		"isDirectoryBucket": backend.isDirectoryBucket,
	}).Info("S3 bucket type detected")

	if backend.isDirectoryBucket {
		logrus.Info("Using S3 Express directory bucket - ACL and DSSE-KMS encryption will be skipped if configured")
	}

	if c.ACL != "" {
		backend.acl = c.ACL
	}

	return backend, nil
}

func normalizeEndpoint(endpoint string) string {
	if endpoint == "" || strings.Contains(endpoint, "://") {
		return endpoint
	}
	return "https://" + endpoint
}

// Get writes downloaded content to the given writer.
func (b *Backend) Get(ctx context.Context, p string, w io.Writer) error {
	in := &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(p),
	}

	errCh := make(chan error)

	go func() {
		defer close(errCh)

		out, err := b.client.GetObject(ctx, in)
		if err != nil {
			logrus.WithError(err).WithField("key", p).Error("GetObject failed")
			errCh <- fmt.Errorf("get the object, %w", err)
			return
		}

		defer internal.CloseWithErrLogf(b.logger, out.Body, "response body, close defer")

		bytesWritten, err := io.Copy(w, out.Body)
		if err != nil {
			logrus.WithError(err).WithField("key", p).Error("Failed to copy object")
			errCh <- fmt.Errorf("copy the object, %w", err)
			return
		}

		logrus.WithFields(logrus.Fields{
			"key":          p,
			"bytesWritten": bytesWritten,
		}).Debug("Downloaded object successfully")
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Put uploads contents of the given reader.
// Uses the S3 manager to handle non-seekable streams (e.g. pipe readers from
// archive compression) by buffering them into parts, matching the v1
// s3manager.Uploader behaviour.
func (b *Backend) Put(ctx context.Context, p string, r io.Reader) error {
	in := &s3.PutObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(p),
		Body:   r,
	}

	// S3 Express directory buckets do not support ACL. Skip and warn instead of
	// letting the upload fail with an opaque AWS API error.
	if b.acl != "" {
		if b.isDirectoryBucket {
			logrus.WithFields(logrus.Fields{
				"bucket": b.bucket,
				"acl":    b.acl,
			}).Warn("ACL is not supported for S3 Express directory buckets; ignoring ACL setting")
		} else {
			in.ACL = s3types.ObjectCannedACL(b.acl)
		}
	}

	// S3 Express directory buckets do not support DSSE-KMS encryption. AES256
	// and aws:kms are supported and applied normally.
	if b.encryption != "" {
		if b.isDirectoryBucket && strings.EqualFold(b.encryption, "aws:dsse-kms") {
			logrus.WithFields(logrus.Fields{
				"bucket":     b.bucket,
				"encryption": b.encryption,
			}).Warn("DSSE-KMS encryption is not supported for S3 Express directory buckets; ignoring encryption setting")
		} else {
			in.ServerSideEncryption = s3types.ServerSideEncryption(b.encryption)
		}
	}

	result, err := b.uploader.Upload(ctx, in)
	if err != nil {
		logrus.WithError(err).WithField("key", p).Error("Upload failed")
		return fmt.Errorf("put the object, %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"key":  p,
		"etag": result.ETag,
	}).Debug("Upload succeeded")

	return nil
}

// Exists checks if object already exists.
func (b *Backend) Exists(ctx context.Context, p string) (bool, error) {
	in := &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(p),
	}

	out, err := b.client.HeadObject(ctx, in)
	if err != nil {
		var apiErr smithy.APIError
		if ok := errors.As(err, &apiErr); ok {
			code := apiErr.ErrorCode()
			if code == "NotFound" || code == "404" || code == "NoSuchKey" {
				return false, nil
			}
		}
		var nsk *s3types.NoSuchKey
		if errors.As(err, &nsk) {
			return false, nil
		}
		return false, fmt.Errorf("head the object, %w", err)
	}

	return out.ETag != nil && *out.ETag != "", nil
}

// List contents of the given directory by given key from remote storage.
func (b *Backend) List(ctx context.Context, p string) ([]common.FileEntry, error) {
	in := &s3.ListObjectsV2Input{
		Bucket: aws.String(b.bucket),
		Prefix: aws.String(p),
	}

	var entries []common.FileEntry

	paginator := s3.NewListObjectsV2Paginator(b.client, in)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return entries, err
		}
		for _, item := range page.Contents {
			entries = append(entries, common.FileEntry{
				Path:         *item.Key,
				Size:         *item.Size,
				LastModified: *item.LastModified,
			})
		}
	}

	return entries, nil
}

func assumeRole(ctx context.Context, roleArn, roleSessionName, externalID, region string) aws.CredentialsProvider {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		logrus.WithError(err).Fatal("Failed to load config for role assumption")
	}

	stsSvc := sts.NewFromConfig(cfg)
	provider := stscreds.NewAssumeRoleProvider(stsSvc, roleArn, func(o *stscreds.AssumeRoleOptions) {
		o.RoleSessionName = roleSessionName
		o.Duration = time.Hour
		if externalID != "" {
			o.ExternalID = aws.String(externalID)
		}
	})

	return aws.NewCredentialsCache(provider)
}

// s3HeadBucketAPI defines the minimal interface needed for bucket type detection.
type s3HeadBucketAPI interface {
	HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
}

// detectDirectoryBucket determines whether the bucket is an S3 Express directory
// bucket. It uses the AWS-enforced "--x-s3" suffix as a zero-cost fast path
// Only when the suffix matches does it make a HeadBucket call to
// validate via bucket metadata, ensuring non-AWS providers (MinIO, R2, etc.)
// that happen to use the same suffix are never misidentified.
func detectDirectoryBucket(ctx context.Context, client s3HeadBucketAPI, bucket string) bool {
	// Fast path: skip the API call entirely for regular buckets.
	if !strings.HasSuffix(bucket, "--x-s3") {
		return false
	}

	out, err := client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		logrus.WithError(err).WithField("bucket", bucket).
			Debug("HeadBucket failed; treating bucket as general-purpose")
		return false
	}

	return out.BucketLocationType == s3types.LocationTypeAvailabilityZone ||
		out.BucketLocationType == s3types.LocationTypeLocalZone
}

func assumeRoleWithWebIdentity(ctx context.Context, roleArn, roleSessionName, webIdentityToken, region string) (aws.CredentialsProvider, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %v", err)
	}

	stsSvc := sts.NewFromConfig(cfg)
	duration := int64(time.Hour / time.Second)
	result, err := stsSvc.AssumeRoleWithWebIdentity(ctx, &sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          aws.String(roleArn),
		RoleSessionName:  aws.String(roleSessionName),
		WebIdentityToken: aws.String(webIdentityToken),
		DurationSeconds:  aws.Int32(int32(duration)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to assume role with web identity: %v", err)
	}
	if result.Credentials == nil {
		return nil, fmt.Errorf("STS AssumeRoleWithWebIdentity returned nil credentials")
	}

	return credentials.NewStaticCredentialsProvider(
		*result.Credentials.AccessKeyId,
		*result.Credentials.SecretAccessKey,
		*result.Credentials.SessionToken,
	), nil
}
