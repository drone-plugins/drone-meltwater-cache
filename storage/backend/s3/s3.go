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
	client     *s3.S3
}

// New creates a new S3 backend with lazy-loaded credentials.
func New(l log.Logger, c Config, debug bool) (*Backend, error) {

	conf := &aws.Config{
		Region:           aws.String(c.Region),
		Endpoint:         &c.Endpoint,
		DisableSSL:       aws.Bool(strings.HasPrefix(c.Endpoint, "http://")),
		S3ForcePathStyle: aws.Bool(c.PathStyle),
	}

	// Create the initial session
	sess, err := session.NewSession(conf)
	if err != nil {
		logrus.WithError(err).Error("Could not instantiate AWS session")
		return nil, fmt.Errorf("AWS session creation failed: %w", err)
	}
	// Inject credentials after session creation
	if c.Key != "" && c.Secret != "" {
		logrus.Info("Using static credentials (Key/Secret provided)")
		conf.Credentials = credentials.NewStaticCredentials(c.Key, c.Secret, "")
	} else if c.AssumeRoleARN != "" {
		// Use OIDC Token or assume role
		if c.OIDCTokenID != "" {
			logrus.Info("Attempting to assume role with OIDC")
			creds, err := assumeRoleWithWebIdentity(c.AssumeRoleARN, c.AssumeRoleSessionName, c.OIDCTokenID)
			if err != nil {
				logrus.WithError(err).Error("Failed to assume role with OIDC")
				return nil, err
			}
			conf.Credentials = creds
			logrus.Info("Successfully assumed role with OIDC")
		} else {
			conf.Credentials = assumeRole(c.AssumeRoleARN, c.AssumeRoleSessionName, c.ExternalID)
		}
	} else {
		logrus.Warn("No AWS credentials provided or role assumed; using default machine credentials for AWS requests")
	}

	var client *s3.S3

	// If UserRoleArn is set, create a new session and assume the role
	if len(c.UserRoleArn) > 0 {
		logrus.WithFields(logrus.Fields{
			"UserRoleArn":        c.UserRoleArn,
			"UserRoleExternalID": c.UserRoleExternalID,
		}).Info("Setting up credentials with UserRoleArn")

		creds := stscreds.NewCredentials(sess, c.UserRoleArn, func(provider *stscreds.AssumeRoleProvider) {
			if c.UserRoleExternalID != "" {
				logrus.WithField("ExternalID", c.UserRoleExternalID).Info("Setting up creds with UserRoleExternalID")
				provider.ExternalID = aws.String(c.UserRoleExternalID)
			}
		})

		// Update the config with the assumed role credentials, reuse the session
		conf.Credentials = creds
		client = s3.New(sess, conf)
		logrus.Info("Created S3 client with assumed user role")
		// }

	} else {
		// Use the original session for the S3 client if no UserRoleArn is set
		client = s3.New(sess, conf)
		logrus.Info("Created S3 client with default session")
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

		out, err := b.client.GetObjectWithContext(ctx, in)
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

// AssumeRole logic
func assumeRole(roleArn, roleSessionName, externalID string) *credentials.Credentials {

	sess, err := session.NewSession()
	if err != nil {
		logrus.WithError(err).Error("Failed to create session for role assumption")
		return nil
	}

	stsClient := sts.New(sess)
	roleProvider := &stscreds.AssumeRoleProvider{
		Client:          stsClient,
		RoleARN:         roleArn,
		RoleSessionName: roleSessionName,
		Duration:        time.Hour, // 1-hour session
	}

	if externalID != "" {
		roleProvider.ExternalID = aws.String(externalID)
	}

	creds := credentials.NewCredentials(roleProvider)
	
	return creds
}

func assumeRoleWithWebIdentity(roleArn, roleSessionName, webIdentityToken string) (*credentials.Credentials, error) {
	// Create a new session
	sess, err := session.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %v", err)
	}

	// Create a new STS client
	svc := sts.New(sess)

	// Prepare the input parameters for the STS call
	duration := int64(time.Hour / time.Second)
	input := &sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          aws.String(roleArn),
		RoleSessionName:  aws.String(roleSessionName),
		WebIdentityToken: aws.String(webIdentityToken),
		DurationSeconds:  aws.Int64(duration),
	}

	// Call the AssumeRoleWithWebIdentity function
	result, err := svc.AssumeRoleWithWebIdentity(input)
	if err != nil {
		return nil, fmt.Errorf("failed to assume role with web identity: %v", err)
	}

	// Create credentials using the response from STS
	newCreds := credentials.NewStaticCredentials(*result.Credentials.AccessKeyId, *result.Credentials.SecretAccessKey, *result.Credentials.SessionToken)
	return newCreds, nil
}
