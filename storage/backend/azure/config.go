package azure

import "time"

// Config is a structure to store Azure backend configuration.
type Config struct {
	// Authentication - Service Principal (Priority 1)
	ClientID     string // Azure Application (Client) ID
	ClientSecret string // Azure Application Secret
	TenantID     string // Azure Tenant ID

	// Authentication - Shared Key (Priority 2, fallback)
	AccountName string // Azure Storage Account Name
	AccountKey  string // Azure Storage Account Key

	// Storage Configuration
	ContainerName    string
	BlobStorageURL    string
	Azurite           bool
	MaxRetryRequests  int
	Timeout           time.Duration
}
