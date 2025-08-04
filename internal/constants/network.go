// Package constants provides shared constants used throughout the application.
package constants

// Network ports.
const (
	// DefaultAPIPort is the standard HTTP port for API endpoints.
	DefaultAPIPort = 80

	// DefaultMediaPort is the port for streaming endpoints (MUST be 5004 for HDHomeRun compatibility).
	DefaultMediaPort = 5004
)

// HTTP content types.
const (
	// ContentTypeJSON is the MIME type for JSON responses.
	ContentTypeJSON = "application/json"
)
