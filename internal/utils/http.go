// Package utils provides utility functions shared across the application.
package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/attaebra/hdhr-proxy/internal/constants"
	"github.com/attaebra/hdhr-proxy/internal/interfaces"
	"github.com/attaebra/hdhr-proxy/internal/logger"
)

// ClientWrapper wraps http.Client to implement our interfaces.Client interface.
type ClientWrapper struct {
	*http.Client
}

// Ensure ClientWrapper implements the Client interface.
var _ interfaces.Client = (*ClientWrapper)(nil)

// HTTPClient creates a high-performance HTTP client with connection pooling.
func HTTPClient(timeout time.Duration) interfaces.Client {
	// Connection pooling for HDHomeRun proxy usage
	transport := &http.Transport{
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 20,
		MaxConnsPerHost:     30,
		IdleConnTimeout:     60 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   3 * time.Second,
			KeepAlive: 15 * time.Second,
		}).DialContext,
		ResponseHeaderTimeout: 5 * time.Second,
		ExpectContinueTimeout: 500 * time.Millisecond,
		DisableCompression:    true,
		ForceAttemptHTTP2:     false,
		DisableKeepAlives:     false,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	logger.Debug("🌐 Created HTTP client",
		logger.Duration("timeout", timeout))
	return &ClientWrapper{Client: client}
}

// HTTPClientWithTimeout creates a client with custom timeout using the same transport.
func HTTPClientWithTimeout(timeout time.Duration) interfaces.Client {
	// Use the same transport configuration as HTTPClient
	return HTTPClient(timeout)
}

// BuildAPIURL constructs a URL for API endpoints with the appropriate port.
func BuildAPIURL(host, path string) string {
	// Ensure path starts with a slash
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	url := fmt.Sprintf("http://%s%s", host, path)
	logger.Debug("🔗 Built API URL",
		logger.String("url", url))
	return url
}

// BuildMediaURL constructs a URL for media streaming endpoints.
func BuildMediaURL(host, channel string) string {
	url := fmt.Sprintf("http://%s:%d/auto/v%s", host, constants.DefaultMediaPort, channel)
	logger.Debug("🎬 Built media URL",
		logger.String("url", url))
	return url
}

// SendRequest sends an HTTP request with timing and logging.
func SendRequest(client *http.Client, method, url string, body io.Reader) (*http.Response, error) {
	logger.Debug("📡 Sending HTTP request",
		logger.String("method", method),
		logger.String("url", url))

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

	logger.Debug("📨 Received HTTP response",
		logger.String("url", url),
		logger.Duration("elapsed", elapsed),
		logger.Int("status_code", resp.StatusCode))

	return resp, nil
}

// SendRequestWithContext sends an HTTP request with a context and logs timing information.
func SendRequestWithContext(client *http.Client, req *http.Request) (*http.Response, error) {
	logger.Debug("📡 Sending HTTP request with context",
		logger.String("method", req.Method),
		logger.String("url", req.URL.String()))

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		return nil, LogAndWrapError(err, "Request to %s failed after %dms",
			req.URL.String(), elapsed.Milliseconds())
	}

	logger.Debug("📨 Received HTTP response with context",
		logger.String("url", req.URL.String()),
		logger.Duration("elapsed", elapsed),
		logger.Int("status_code", resp.StatusCode))

	return resp, nil
}

// WriteJSONResponse writes a JSON response with appropriate headers.
func WriteJSONResponse(w http.ResponseWriter, data interface{}) error {
	w.Header().Set("Content-Type", constants.ContentTypeJSON)

	encoder := json.NewEncoder(w)
	err := encoder.Encode(data)
	if err != nil {
		logger.Error("❌ Failed to encode JSON response",
			logger.ErrorField("error", err))
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
		logger.Warn("⚠️  Error closing resource",
			logger.String("description", description),
			logger.ErrorField("error", err))
	} else {
		logger.Debug("✅ Successfully closed resource",
			logger.String("description", description))
	}
}

// TimeOperation times an operation and logs its duration.
func TimeOperation(description string) func() {
	start := time.Now()
	logger.Debug("⏱️  Starting operation",
		logger.String("operation", description))

	return func() {
		elapsed := time.Since(start)
		logger.Debug("✅ Completed operation",
			logger.String("operation", description),
			logger.Duration("elapsed", elapsed))
	}
}
