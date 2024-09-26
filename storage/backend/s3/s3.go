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
	// "github.com/go-kit/kit/log/level"
	"github.com/sirupsen/logrus"

	"github.com/meltwater/drone-cache/internal"
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
    logrus.WithFields(logrus.Fields{
        "UserRoleExternalID": c.UserRoleExternalID,
        "AssumeRoleARN":      c.AssumeRoleARN,
        "UserRoleArn":        c.UserRoleArn,
    }).Info("Initializing new Backend")

    conf := &aws.Config{
        Region:           aws.String(c.Region),
        Endpoint:         &c.Endpoint,
        DisableSSL:       aws.Bool(strings.HasPrefix(c.Endpoint, "http://")),
        S3ForcePathStyle: aws.Bool(c.PathStyle),
    }

    // Setup initial credentials
    if c.Key != "" && c.Secret != "" {
        logrus.Info("Using static credentials")
        conf.Credentials = credentials.NewStaticCredentials(c.Key, c.Secret, "")
    }

    // Create initial session
    sess, err := session.NewSession(conf)
    if err != nil {
        logrus.WithError(err).Error("Could not create initial session")
        return nil, err
    }

    // Handle role assumption
    if c.AssumeRoleARN != "" {
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
        // Create a new session with the assumed role
        sess, err = session.NewSession(conf)
        if err != nil {
            logrus.WithError(err).Error("Could not create session with assumed role")
            return nil, err
        }
    }

    var client *s3.S3
    if len(c.UserRoleArn) > 0 {
        logrus.Info("Setting up credentials with UserRoleArn")
        creds := stscreds.NewCredentials(sess, c.UserRoleArn, func(provider *stscreds.AssumeRoleProvider) {
            if c.UserRoleExternalID != "" {
                logrus.WithField("ExternalID", c.UserRoleExternalID).Info("Setting up creds with UserRoleExternalID")
                provider.ExternalID = aws.String(c.UserRoleExternalID)
            }
        })
        // Create a new config with the assumed role credentials
        clientConf := aws.Config{
            Credentials: creds,
            Region:      conf.Region,
            Endpoint:    conf.Endpoint,
        }
        client = s3.New(sess, &clientConf)
        logrus.Info("Created S3 client with assumed user role")
    } else {
        client = s3.New(sess)
        logrus.Info("Created S3 client without user role")
    }

    logrus.WithField("Client", client).Info("New Client set here.")

    backend := &Backend{
        logger:     l,
        bucket:     c.Bucket,
        encryption: c.Encryption,
        client:     client,
    }

    if c.ACL != "" {
        backend.acl = c.ACL
    }

    logrus.WithFields(logrus.Fields{
        "Bucket":     backend.bucket,
        "Encryption": backend.encryption,
        "ACL":        backend.acl,
    }).Info("Backend initialized")

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

func assumeRole(roleArn, roleSessionName, externalID string) *credentials.Credentials {
    logrus.WithFields(logrus.Fields{
        "roleArn":         roleArn,
        "roleSessionName": roleSessionName,
        "externalID":      externalID,
    }).Info("Attempting to assume role")

    sess, _ := session.NewSession()
    client := sts.New(sess)
    duration := time.Hour * 1
    stsProvider := &stscreds.AssumeRoleProvider{
        Client:          client,
        Duration:        duration,
        RoleARN:         roleArn,
        RoleSessionName: roleSessionName,
    }

    if externalID != "" {
        stsProvider.ExternalID = &externalID
        logrus.WithField("externalID", externalID).Info("Using external ID for assume role")
    }

    creds := credentials.NewCredentials(stsProvider)

    // Test the credentials
    _, err := creds.Get()
    if err != nil {
        logrus.WithError(err).Error("Failed to assume role")
    } else {
        logrus.Info("Successfully assumed role")
    }

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
