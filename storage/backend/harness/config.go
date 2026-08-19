package harness

// Config is a structure to store harness backend configuration.
type Config struct {
	AccountID     string
	Token         string
	ServerBaseURL string
	OrgID         string
	ProjectID     string
	PipelineID    string
	StageID       string
	// CacheType selects unified cache-type aware APIs when non-empty (e.g., "step").
	CacheType              string
	MultipartChunkSize     int
	MultipartMaxUploadSize int
	MultipartThresholdSize int
	MultipartEnabled       string
}
