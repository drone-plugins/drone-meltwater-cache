package s3

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"

	"github.com/meltwater/drone-cache/storage/common"
)

// Backend implements storage.Backend for AWs S3.
type Backend struct {
	logger log.Logger

	bucket     string
	acl        string
	encryption string
	client     *s3.S3
}

// New creates an S3 backend.
func New(l log.Logger, c Config, debug bool) (*Backend, error) {
    conf := &aws.Config{
        Region:           aws.String(c.Region),
        Endpoint:         &c.Endpoint,
        DisableSSL:       aws.Bool(strings.HasPrefix(c.Endpoint, "http://")),
        S3ForcePathStyle: aws.Bool(c.PathStyle),
    }

    sess, err := session.NewSession(conf)
    if err != nil {
        level.Warn(l).Log("msg", "could not instantiate session", "error", err)
        return nil, err
    }

    if c.Key != "" && c.Secret != "" {
        conf.Credentials = credentials.NewStaticCredentials(c.Key, c.Secret, "")
    } else if c.AssumeRoleARN != "" {
        if c.OIDCTokenID != "" {
            // Assume role with OIDC
            conf.Credentials, err = assumeRoleWithWebIdentity(sess, c.AssumeRoleARN, c.AssumeRoleSessionName, c.OIDCTokenID)
			if err != nil {
				level.Error(l).Log("component", "s3-backend", "function", "assumeRoleWithWebIdentity", "msg", "failed to assume role with OIDC", "error", err)
				return nil, err
			}
        } else {
            conf.Credentials = assumeRole(c.AssumeRoleARN, c.AssumeRoleSessionName)
        }
    } else {
        level.Warn(l).Log("msg", "AWS credentials not provided (falling back to anonymous credentials)")
    }

    // Create the S3 client with the configured region and credentials
    client := s3.New(sess)

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

    errCh := make(chan error, 1)  // Buffer the channel to prevent blocking in case of error

    go func() {
        defer close(errCh)
        out, err := b.client.GetObjectWithContext(ctx, in)
        if err != nil {
            errCh <- fmt.Errorf("get the object, %w", err)
            return
        }
        defer out.Body.Close()  // Ensure the body is closed after reading

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
	var (
		uploader = s3manager.NewUploaderWithClient(b.client)
		in       = &s3manager.UploadInput{
			Bucket: aws.String(b.bucket),
			Key:    aws.String(p),
			ACL:    aws.String(b.acl),
			Body:   r,
		}
	)

	if b.encryption != "" {
		in.ServerSideEncryption = aws.String(b.encryption)
	}

	if _, err := uploader.UploadWithContext(ctx, in); err != nil {
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

	out, err := b.client.HeadObjectWithContext(ctx, in)
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok && awsErr.Code() == s3.ErrCodeNoSuchKey || awsErr.Code() == "NotFound" {
			return false, nil
		}

		return false, fmt.Errorf("head the object, %w", err)
	}

	// Normally if file not exists it will be already detected by error above but in some cases
	// Minio can return success status for without ETag, detect that here.
	return *out.ETag != "", nil
}

// List contents of the given directory by given key from remote storage.
func (b *Backend) List(ctx context.Context, p string) ([]common.FileEntry, error) {
	in := &s3.ListObjectsInput{
		Bucket: aws.String(b.bucket),
		Prefix: aws.String(p),
	}

	var entries []common.FileEntry

	err := b.client.ListObjectsPagesWithContext(ctx, in, func(page *s3.ListObjectsOutput, lastPage bool) bool {
		for _, item := range page.Contents {
			entries = append(entries, common.FileEntry{
				Path:         *item.Key,
				Size:         *item.Size,
				LastModified: *item.LastModified,
			})
		}
		return !lastPage
	})

	return entries, err
}

func assumeRole(roleArn, roleSessionName string) *credentials.Credentials {
	client := sts.New(session.New()) // nolint:staticcheck
	duration := time.Hour * 1
	stsProvider := &stscreds.AssumeRoleProvider{
		Client:          client,
		Duration:        duration,
		RoleARN:         roleArn,
		RoleSessionName: roleSessionName,
	}

	return credentials.NewCredentials(stsProvider)
}

func assumeRoleWithWebIdentity(sess *session.Session, roleArn, roleSessionName, idToken string) (*credentials.Credentials, error) {
    svc := sts.New(sess)
    input := &sts.AssumeRoleWithWebIdentityInput{
        RoleArn:          aws.String(roleArn),
        RoleSessionName:  aws.String(roleSessionName),
        WebIdentityToken: aws.String(idToken),
    }
    result, err := svc.AssumeRoleWithWebIdentity(input)
    if err != nil {
        return nil, fmt.Errorf("failed to assume role with web identity: %w", err)
    }
    return credentials.NewStaticCredentials(*result.Credentials.AccessKeyId, *result.Credentials.SecretAccessKey, *result.Credentials.SessionToken), nil
}
