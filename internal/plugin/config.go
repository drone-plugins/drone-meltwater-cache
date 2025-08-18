package plugin

import (
	"time"

	"github.com/meltwater/drone-cache/storage/backend/azure"
	"github.com/meltwater/drone-cache/storage/backend/filesystem"
	"github.com/meltwater/drone-cache/storage/backend/gcs"
	"github.com/meltwater/drone-cache/storage/backend/harness"
	"github.com/meltwater/drone-cache/storage/backend/s3"
	"github.com/meltwater/drone-cache/storage/backend/sftp"
)

// Config plugin-specific parameters and secrets.
type Config struct {
	ArchiveFormat    string
	Backend          string
	CacheKeyTemplate string
	MetricsFile      string
	RemoteRoot       string
	LocalRoot        string
	AccountID        string

	// Modes
	Debug               bool
	Rebuild             bool
	Restore             bool
	AutoDetect          bool
	AutoDetectEarlyExit bool

	// Optional
	SkipSymlinks               bool
	Override                   bool
	FailRestoreIfKeyNotPresent bool
	CompressionLevel           int
	StorageOperationTimeout    time.Duration
	EnableCacheKeySeparator    bool
	StrictKeyMatching          bool `envconfig:"PLUGIN_STRICT_KEY_MATCHING" default:"true"`
	PreserveMetadata bool

	Mount []string

	// Backend
	S3         s3.Config
	FileSystem filesystem.Config
	SFTP       sftp.Config
	Azure      azure.Config
	GCS        gcs.Config
	Harness    harness.Config
}
