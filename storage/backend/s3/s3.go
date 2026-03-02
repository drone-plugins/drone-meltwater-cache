package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/go-kit/kit/log"
	"github.com/sirupsen/logrus"

	"github.com/meltwater/drone-cache/internal"
	"github.com/meltwater/drone-cache/storage/common"
)

// Backend implements storage.Backend for AWS S3.
type Backend struct {
	logger log.Logger

	bucket     string
	acl        string
	encryption string
	client     *s3.Client
}

// New creates a new S3 backend with lazy-loaded credentials.
func New(l log.Logger, c Config, debug bool) (*Backend, error) {
	ctx := context.Background()

	// Build config options
	var optFns []func(*config.LoadOptions) error

	optFns = append(optFns, config.WithRegion(c.Region))

	// Handle custom endpoint (for S3-compatible services like MinIO)
	if c.Endpoint != "" {
		customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:               c.Endpoint,
				HostnameImmutable: true,
				SigningRegion:     c.Region,
			}, nil
		})
		optFns = append(optFns, config.WithEndpointResolverWithOptions(customResolver))
	}

	// Handle static credentials
	if c.Key != "" && c.Secret != "" {
		logrus.Info("Using static credentials (Key/Secret provided)")
		optFns = append(optFns, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(c.Key, c.Secret, ""),
		))
	}

	// Load the base config
	cfg, err := config.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		logrus.WithError(err).Error("Could not load AWS config")
		return nil, fmt.Errorf("AWS config loading failed: %w", err)
	}

	// Handle role assumption
	if c.AssumeRoleARN != "" {
		if c.OIDCTokenID != "" {
			logrus.Info("Attempting to assume role with OIDC")
			stsClient := sts.NewFromConfig(cfg)
			webIdentityProvider := stscreds.NewWebIdentityRoleProvider(
				stsClient,
				c.AssumeRoleARN,
				stscreds.IdentityTokenFile(c.OIDCTokenID),
				func(o *stscreds.WebIdentityRoleOptions) {
					if c.AssumeRoleSessionName != "" {
						o.RoleSessionName = c.AssumeRoleSessionName
					}
				},
			)
			cfg.Credentials = aws.NewCredentialsCache(webIdentityProvider)
			logrus.Info("Successfully configured role assumption with OIDC")
		} else {
			logrus.Info("Attempting to assume role")
			stsClient := sts.NewFromConfig(cfg)
			assumeRoleProvider := stscreds.NewAssumeRoleProvider(
				stsClient,
				c.AssumeRoleARN,
				func(o *stscreds.AssumeRoleOptions) {
					if c.AssumeRoleSessionName != "" {
						o.RoleSessionName = c.AssumeRoleSessionName
					}
					if c.ExternalID != "" {
						o.ExternalID = aws.String(c.ExternalID)
					}
				},
			)
			cfg.Credentials = aws.NewCredentialsCache(assumeRoleProvider)
			logrus.Info("Successfully configured role assumption")
		}
	} else {
		logrus.Warn("No AWS credentials provided or role assumed; using default machine credentials for AWS requests")
	}

	// Create S3 client options
	s3Opts := []func(*s3.Options){}

	if c.PathStyle {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	// Initialize client
	client := s3.NewFromConfig(cfg, s3Opts...)

	// Handle secondary role assumption if UserRoleArn is provided
	if len(c.UserRoleArn) > 0 {
		logrus.WithFields(logrus.Fields{
			"UserRoleArn":        c.UserRoleArn,
			"UserRoleExternalID": c.UserRoleExternalID,
		}).Info("Setting up credentials with UserRoleArn")

		stsClient := sts.NewFromConfig(cfg)
		userRoleProvider := stscreds.NewAssumeRoleProvider(
			stsClient,
			c.UserRoleArn,
			func(o *stscreds.AssumeRoleOptions) {
				if c.UserRoleExternalID != "" {
					logrus.WithField("ExternalID", c.UserRoleExternalID).Info("Setting up creds with UserRoleExternalID")
					o.ExternalID = aws.String(c.UserRoleExternalID)
				}
			},
		)
		cfg.Credentials = aws.NewCredentialsCache(userRoleProvider)

		// Create new client with updated credentials
		client = s3.NewFromConfig(cfg, s3Opts...)
		logrus.Info("Created S3 client with assumed user role")
	}

	logrus.WithFields(logrus.Fields{
		"Bucket": c.Bucket,
		"Region": c.Region,
	}).Info("New S3 client configured")

	backend := &Backend{
		logger:     l,
		bucket:     c.Bucket,
		encryption: c.Encryption,
		client:     client,
	}

	if c.ACL != "" {
		backend.acl = c.ACL
	}

	return backend, nil
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
			errCh <- fmt.Errorf("get the object, %w", err)
			return
		}

		defer internal.CloseWithErrLogf(b.logger, out.Body, "response body, close defer")

		_, err = io.Copy(w, out.Body)
		if err != nil {
			errCh <- fmt.Errorf("copy the object, %w", err)
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Put uploads contents of the given reader.
func (b *Backend) Put(ctx context.Context, p string, r io.Reader) error {
	uploader := manager.NewUploader(b.client)

	in := &s3.PutObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(p),
		Body:   r,
	}

	if b.acl != "" {
		in.ACL = types.ObjectCannedACL(b.acl)
	}

	if b.encryption != "" {
		in.ServerSideEncryption = types.ServerSideEncryption(b.encryption)
	}

	if _, err := uploader.Upload(ctx, in); err != nil {
		return fmt.Errorf("put the object, %w", err)
	}

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
		var notFound *types.NotFound
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &notFound) || errors.As(err, &noSuchKey) || strings.Contains(err.Error(), "NotFound") {
			return false, nil
		}

		return false, fmt.Errorf("head the object, %w", err)
	}

	// Normally if file not exists it will be already detected by error above but in some cases
	// Minio can return success status without ETag, detect that here.
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
			return nil, fmt.Errorf("list objects, %w", err)
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
