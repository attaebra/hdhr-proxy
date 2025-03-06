// Package utils provides utility functions shared across the application.
package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/attaebra/hdhr-proxy/internal/constants"
	"github.com/attaebra/hdhr-proxy/internal/logger"
)

// BuildAPIURL constructs a URL for API endpoints with the appropriate port.
func BuildAPIURL(host, path string) string {
	// Ensure path starts with a slash
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	url := fmt.Sprintf("http://%s%s", host, path)
	logger.Debug("Built API URL: %s", url)
	return url
}

// BuildMediaURL constructs a URL for media streaming endpoints.
func BuildMediaURL(host, channel string) string {
	url := fmt.Sprintf("http://%s:%d/auto/v%s", host, constants.DefaultMediaPort, channel)
	logger.Debug("Built media URL: %s", url)
	return url
}

// SendRequest sends an HTTP request with timing and logging.
func SendRequest(client *http.Client, method, url string, body io.Reader) (*http.Response, error) {
	logger.Debug("Sending %s request to: %s", method, url)

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, LogAndWrapError(err, "Failed to create request for %s", url)
	}

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		return nil, LogAndWrapError(err, "Request to %s failed after %dms", url, elapsed.Milliseconds())
	}

	logger.Debug("Received response from %s in %dms (status: %d)",
		url, elapsed.Milliseconds(), resp.StatusCode)

	return resp, nil
}

// SendRequestWithContext sends an HTTP request with a context and logs timing information.
func SendRequestWithContext(client *http.Client, req *http.Request) (*http.Response, error) {
	logger.Debug("Sending %s request to: %s", req.Method, req.URL.String())

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		return nil, LogAndWrapError(err, "Request to %s failed after %dms",
			req.URL.String(), elapsed.Milliseconds())
	}

	logger.Debug("Received response from %s in %dms (status: %d)",
		req.URL.String(), elapsed.Milliseconds(), resp.StatusCode)

	return resp, nil
}

// WriteJSONResponse writes a JSON response with appropriate headers.
func WriteJSONResponse(w http.ResponseWriter, data interface{}) error {
	w.Header().Set("Content-Type", constants.ContentTypeJSON)

	encoder := json.NewEncoder(w)
	err := encoder.Encode(data)
	if err != nil {
		logger.Error("Failed to encode JSON response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return err
	}

	return nil
}

// CloseWithLogging closes an io.Closer with logging.
func CloseWithLogging(closer io.Closer, description string) {
	if closer == nil {
		return
	}

	if err := closer.Close(); err != nil {
		logger.Warn("Error closing %s: %v", description, err)
	} else {
		logger.Debug("Successfully closed %s", description)
	}
}

// TimeOperation times an operation and logs its duration.
func TimeOperation(description string) func() {
	start := time.Now()
	logger.Debug("Starting: %s", description)

	return func() {
		elapsed := time.Since(start)
		logger.Debug("Completed: %s (took %dms)", description, elapsed.Milliseconds())
	}
}
