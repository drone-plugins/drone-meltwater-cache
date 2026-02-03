package azure

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"

	"github.com/meltwater/drone-cache/internal"
	"github.com/meltwater/drone-cache/storage/common"
)

// time.Time is used in common.FileEntry.LastModified
var _ time.Time

const (
	// DefaultBlobMaxRetryRequests Default value for Azure Blob Storage Max Retry Requests.
	DefaultBlobMaxRetryRequests = 4

	defaultBufferSize = 3 * 1024 * 1024
	defaultMaxBuffers = 4
)

// Backend implements storage.Backend for Azure Blob Storage.
type Backend struct {
	logger        log.Logger
	cfg           Config
	client        *azblob.Client
	containerName string
}

// New creates an AzureBlob backend.
func New(l log.Logger, c Config) (*Backend, error) {
	// Set default for MaxRetryRequests if not provided
	if c.MaxRetryRequests == 0 {
		c.MaxRetryRequests = DefaultBlobMaxRetryRequests
	}

	// Validate container name is provided
	if c.ContainerName == "" {
		return nil, errors.New("azure container name is required")
	}

	var accountName string
	var cred azcore.TokenCredential
	var err error

	// Authentication: Priority 0 - OIDC (highest priority)
	if c.OIDCTokenID != "" && c.TenantID != "" {
		level.Info(l).Log("msg", "using OIDC token authentication", "tenantID", c.TenantID)

		// For OIDC, we need AccountName and ClientID to construct the URL and credential
		if c.AccountName == "" {
			return nil, errors.New("azure account name is required when using OIDC authentication")
		}
		if c.ClientID == "" {
			return nil, errors.New("azure client ID is required when using OIDC authentication")
		}
		accountName = c.AccountName

		// Use ClientAssertionCredential with OIDC token as the assertion
		// The OIDC token from the CI/CD system is used directly as the client assertion
		oidcToken := c.OIDCTokenID
		getAssertion := func(ctx context.Context) (string, error) {
			return oidcToken, nil
		}

		// Create ClientAssertionCredential using OIDC token as assertion
		cred, err = azidentity.NewClientAssertionCredential(c.TenantID, c.ClientID, getAssertion, nil)
		if err != nil {
			return nil, fmt.Errorf("azure, failed to create OIDC client assertion credential, %w", err)
		}

		// Authentication: Priority 1 - Service Principal (ClientID + ClientSecret + TenantID)
	} else if c.ClientID != "" && c.ClientSecret != "" && c.TenantID != "" {
		level.Info(l).Log("msg", "using service principal authentication", "clientID", c.ClientID, "tenantID", c.TenantID)

		// For Service Principal, we need AccountName to construct the URL
		if c.AccountName == "" {
			return nil, errors.New("azure account name is required when using service principal authentication")
		}
		accountName = c.AccountName

		// Create Service Principal credential using the new Azure SDK
		cred, err = azidentity.NewClientSecretCredential(c.TenantID, c.ClientID, c.ClientSecret, nil)
		if err != nil {
			return nil, fmt.Errorf("azure, failed to create service principal credential, %w", err)
		}

	} else if c.AccountName != "" && c.AccountKey != "" {
		// Authentication: Priority 2 - Shared Key (fallback)
		level.Info(l).Log("msg", "using shared key authentication", "accountName", c.AccountName)

		accountName = c.AccountName

		// For Shared Key, we need to create a credential
		// The new SDK uses azblob.NewSharedKeyCredential for shared key auth
		// But we'll use the account name and key to create the client directly
		// Note: The new SDK doesn't have a direct SharedKeyCredential in azidentity
		// We'll construct the service URL and use account key authentication
		cred = nil // Shared key will be handled via service URL

	} else {
		// No valid authentication method found
		return nil, errors.New("azure authentication requires either (OIDCTokenID + TenantID + ClientID), (ClientID + ClientSecret + TenantID), or (AccountName + AccountKey)")
	}

	// Construct blob storage service URL
	var serviceURL string
	if c.Azurite {
		serviceURL = fmt.Sprintf("http://%s/%s", c.BlobStorageURL, accountName)
	} else {
		serviceURL = fmt.Sprintf("https://%s.%s", accountName, c.BlobStorageURL)
	}

	level.Info(l).Log("msg", "constructing blob storage service URL", "url", serviceURL, "accountName", accountName, "blobStorageURL", c.BlobStorageURL)

	// Create Azure Blob Storage client
	var client *azblob.Client
	if c.OIDCTokenID != "" && c.TenantID != "" {
		// OIDC authentication - use credential
		client, err = azblob.NewClient(serviceURL, cred, nil)
		if err != nil {
			return nil, fmt.Errorf("azure, failed to create client with OIDC, %w", err)
		}
	} else if c.ClientID != "" && c.ClientSecret != "" && c.TenantID != "" && c.OIDCTokenID == "" {
		// Service Principal authentication - use credential
		client, err = azblob.NewClient(serviceURL, cred, nil)
		if err != nil {
			return nil, fmt.Errorf("azure, failed to create client with service principal, %w", err)
		}
	} else {
		// Shared Key authentication
		sharedKeyCred, err := azblob.NewSharedKeyCredential(accountName, c.AccountKey)
		if err != nil {
			return nil, fmt.Errorf("azure, invalid shared key credentials, %w", err)
		}
		client, err = azblob.NewClientWithSharedKeyCredential(serviceURL, sharedKeyCred, nil)
		if err != nil {
			return nil, fmt.Errorf("azure, failed to create client with shared key, %w", err)
		}
	}

	// Ensure container exists - try to create it (idempotent operation)
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()

	level.Info(l).Log("msg", "ensuring container exists", "container", c.ContainerName)

	// Try to create container (idempotent - safe to call if container already exists)
	_, err = client.CreateContainer(ctx, c.ContainerName, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			// Check for ContainerAlreadyExists error code
			if respErr.ErrorCode == "ContainerAlreadyExists" {
				// Container already exists - this is expected and fine, continue
				level.Info(l).Log("msg", "container already exists, continuing", "container", c.ContainerName)
				err = nil // Clear error so we can continue
			} else {
				// Other storage error - check if container actually exists
				level.Info(l).Log("msg", "container creation failed with storage error, verifying container exists", "container", c.ContainerName, "errorCode", respErr.ErrorCode)
				// Try to list blobs in the container to verify it exists
				maxResults := int32(1)
				pager := client.NewListBlobsFlatPager(c.ContainerName, &azblob.ListBlobsFlatOptions{MaxResults: &maxResults})
				_, checkErr := pager.NextPage(ctx)
				if checkErr != nil {
					// Container doesn't exist and we can't create it - this is a real error
					level.Error(l).Log("msg", "failed to create or access container", "container", c.ContainerName, "createError", err, "checkError", checkErr)
					return nil, fmt.Errorf("azure, failed to create or access container, createErr: %w, checkErr: %v", err, checkErr)
				}
				// Container exists (GetProperties succeeded) - creation error was transient, continue
				level.Info(l).Log("msg", "container exists (verified), continuing despite creation error", "container", c.ContainerName)
				err = nil // Clear error so we can continue
			}
		} else {
			// Non-storage error - check if container actually exists
			level.Info(l).Log("msg", "container creation failed with non-storage error, verifying container exists", "container", c.ContainerName, "errorType", fmt.Sprintf("%T", err))
			// Try to list blobs in the container to verify it exists
			maxResults := int32(1)
			pager := client.NewListBlobsFlatPager(c.ContainerName, &azblob.ListBlobsFlatOptions{MaxResults: &maxResults})
			_, checkErr := pager.NextPage(ctx)
			if checkErr != nil {
				// Container doesn't exist and we can't create it - this is a real error
				level.Error(l).Log("msg", "failed to create or access container", "container", c.ContainerName, "createError", err, "checkError", checkErr)
				return nil, fmt.Errorf("azure, failed to create or access container, createErr: %w, checkErr: %v", err, checkErr)
			}
			// Container exists (GetProperties succeeded) - creation error was transient, continue
			level.Info(l).Log("msg", "container exists (verified), continuing despite creation error", "container", c.ContainerName)
			err = nil // Clear error so we can continue
		}
	} else {
		level.Info(l).Log("msg", "container created successfully", "container", c.ContainerName)
	}

	// Verify we can proceed (err should be nil at this point if container exists)
	if err != nil {
		level.Error(l).Log("msg", "unexpected error state after container check", "container", c.ContainerName, "error", err)
		return nil, fmt.Errorf("azure, unexpected error after container check, %w", err)
	}

	backend := &Backend{
		logger:        l,
		cfg:           c,
		client:        client,
		containerName: c.ContainerName,
	}
	return backend, nil
}

// Get writes downloaded content to the given writer.
func (b *Backend) Get(ctx context.Context, p string, w io.Writer) (err error) {
	errCh := make(chan error)

	go func() {
		defer close(errCh)

		resp, err := b.client.DownloadStream(ctx, b.containerName, p, nil)
		if err != nil {
			errCh <- fmt.Errorf("get the object, %w", err)
			return
		}

		rc := resp.Body
		defer internal.CloseWithErrLogf(b.logger, rc, "response body, close defer")

		_, err = io.Copy(w, rc)
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
	level.Info(b.logger).Log("msg", "uploading the file with blob", "name", p, "container", b.containerName)

	level.Debug(b.logger).Log("msg", "uploading to blob", "blobName", p)

	// Use UploadStream which handles the stream directly
	_, err := b.client.UploadStream(ctx, b.containerName, p, r, nil)
	if err != nil {
		level.Error(b.logger).Log("msg", "failed to upload blob", "blobName", p, "error", err)
		return fmt.Errorf("put the object, %w", err)
	}

	level.Info(b.logger).Log("msg", "successfully uploaded blob", "name", p)
	return nil
}

// Exists checks if path already exists.
func (b *Backend) Exists(ctx context.Context, p string) (bool, error) {
	level.Info(b.logger).Log("msg", "checking if the object already exists", "name", p)

	// Try to download to check if blob exists (we'll just check the error, not download the content)
	_, err := b.client.DownloadStream(ctx, b.containerName, p, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, fmt.Errorf("check if object exists, %w", err)
	}

	return true, nil
}

// List contents of the given directory by given key from remote storage.
func (b *Backend) List(ctx context.Context, p string) ([]common.FileEntry, error) {
	level.Info(b.logger).Log("msg", "listing blobs", "prefix", p)

	var entries []common.FileEntry

	// Ensure prefix ends with / if it's not empty and doesn't already end with /
	prefix := p
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	// List blobs with pagination
	pager := b.client.NewListBlobsFlatPager(b.containerName, &azblob.ListBlobsFlatOptions{
		Prefix: &prefix,
	})

	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list blobs, %w", err)
		}

		// Process each blob in the segment
		for _, blobInfo := range resp.Segment.BlobItems {
			if blobInfo.Properties.ContentLength != nil && blobInfo.Properties.LastModified != nil {
				entries = append(entries, common.FileEntry{
					Path:         *blobInfo.Name,
					Size:         *blobInfo.Properties.ContentLength,
					LastModified: *blobInfo.Properties.LastModified,
				})
			}
		}
	}

	level.Info(b.logger).Log("msg", "listed blobs", "prefix", p, "count", len(entries))

	return entries, nil
}
