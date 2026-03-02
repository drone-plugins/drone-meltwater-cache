package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
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

	optFns := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(c.Region),
	}

	if c.Key != "" && c.Secret != "" {
		logrus.Info("Using static credentials (Key/Secret provided)")
		optFns = append(optFns, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(c.Key, c.Secret, ""),
		))
	} else if c.Credentials != nil {
		logrus.Info("Using provided credentials")
		optFns = append(optFns, awsconfig.WithCredentialsProvider(c.Credentials))
	} else if c.AssumeRoleARN != "" {
		if c.OIDCTokenID != "" {
			logrus.Info("Attempting to assume role with OIDC")
			creds, err := assumeRoleWithWebIdentity(ctx, c.AssumeRoleARN, c.AssumeRoleSessionName, c.OIDCTokenID)
			if err != nil {
				logrus.WithError(err).Error("Failed to assume role with OIDC")
				return nil, err
			}
			optFns = append(optFns, awsconfig.WithCredentialsProvider(creds))
			logrus.Info("Successfully assumed role with OIDC")
		} else {
			creds := assumeRole(ctx, c.AssumeRoleARN, c.AssumeRoleSessionName, c.ExternalID)
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
	if c.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(c.Endpoint)
			o.UsePathStyle = c.PathStyle
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
	in := &s3.PutObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(p),
		ACL:    s3types.ObjectCannedACL(b.acl),
		Body:   r,
	}

	if b.encryption != "" {
		in.ServerSideEncryption = s3types.ServerSideEncryption(b.encryption)
	}

	if _, err := b.client.PutObject(ctx, in); err != nil {
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

func assumeRole(ctx context.Context, roleArn, roleSessionName, externalID string) aws.CredentialsProvider {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		logrus.WithError(err).Error("Failed to load config for role assumption")
		return nil
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

func assumeRoleWithWebIdentity(ctx context.Context, roleArn, roleSessionName, webIdentityToken string) (aws.CredentialsProvider, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
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

	return credentials.NewStaticCredentialsProvider(
		*result.Credentials.AccessKeyId,
		*result.Credentials.SecretAccessKey,
		*result.Credentials.SessionToken,
	), nil
}
