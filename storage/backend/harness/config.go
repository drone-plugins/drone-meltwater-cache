package harness

// Config is a structure to store harness backend configuration.
type Config struct {
	AccountID              string
	Token                  string
	ServerBaseURL          string
	MultipartChunkSize     int
	MultipartMaxUploadSize int
	MultipartThresholdSize int
	MultipartEnabled       string
}
