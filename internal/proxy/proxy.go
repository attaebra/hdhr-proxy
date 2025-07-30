// Package proxy provides functionality to proxy and transform requests to/from
// the HDHomeRun device. It handles request forwarding, response modification,
// and maintains compatibility with media streaming clients.
package proxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/attaebra/hdhr-proxy/internal/interfaces"
	"github.com/attaebra/hdhr-proxy/internal/logger"
	"github.com/attaebra/hdhr-proxy/internal/utils"
)

// RequestSetup holds the common request setup data.
type RequestSetup struct {
	TargetURL *url.URL
	ProxyReq  *http.Request
}

// Constants for response handling.
const (
	maxInMemorySize = 1024 * 1024 // 1MB - maximum size for in-memory response transformation
)

// HDHRProxy represents an HDHomeRun proxy instance.
type HDHRProxy struct {
	HDHRIP   string
	deviceID string
	Client   interfaces.Client
	logger   interfaces.Logger
}

// Ensure HDHRProxy implements the HDHRProxy interface.
var _ interfaces.Proxy = (*HDHRProxy)(nil)

// NewForTesting creates a new HDHomeRun proxy instance for testing.
// For production use, use New() with dependency injection.
func NewForTesting(hdhrIP string) *HDHRProxy {
	// Create HTTP client with reasonable timeout for API requests
	client := utils.HTTPClient(30 * time.Second)
	// Create logger for testing
	testLogger := logger.NewZapLogger(logger.LevelDebug)

	return &HDHRProxy{
		HDHRIP:   hdhrIP,
		deviceID: "00ABCDEF", // Default device ID, will be updated
		Client:   client,
		logger:   testLogger,
	}
}

// New creates a new HDHomeRun proxy instance with injected dependencies.
func New(hdhrIP string, httpClient interfaces.Client, logger interfaces.Logger) interfaces.Proxy {
	return &HDHRProxy{
		HDHRIP:   hdhrIP,
		deviceID: "00ABCDEF", // Default device ID, will be updated
		Client:   httpClient,
		logger:   logger,
	}
}

// DeviceID returns the current device ID.
func (p *HDHRProxy) DeviceID() string {
	return p.deviceID
}

// GetHDHRIP returns the HDHomeRun IP address.
func (p *HDHRProxy) GetHDHRIP() string {
	return p.HDHRIP
}

// ReverseDeviceID reverses the device ID string.
func (p *HDHRProxy) ReverseDeviceID() string {
	runes := []rune(p.deviceID)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// FetchDeviceID retrieves the actual device ID from the HDHomeRun.
func (p *HDHRProxy) FetchDeviceID() error {
	defer utils.TimeOperation("Fetch device ID")()
	p.logger.Debug("📡 Fetching device ID from HDHomeRun",
		logger.String("hdhr_ip", p.HDHRIP))
	resp, err := p.Client.Get("http://" + p.HDHRIP + "/discover.json")
	if err != nil {
		return utils.LogAndWrapError(err, "failed to connect to HDHomeRun at %s", p.HDHRIP)
	}
	defer resp.Body.Close()

	p.logger.Debug("📨 Received discovery response",
		logger.Int("status_code", resp.StatusCode))

	if resp.StatusCode != http.StatusOK {
		return utils.LogAndWrapError(fmt.Errorf("HTTP status %d", resp.StatusCode), "invalid response from HDHomeRun")
	}

	// Read and parse the response to extract DeviceID
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return utils.LogAndWrapError(err, "failed to read response body")
	}

	// Parse JSON response
	var discovery struct {
		DeviceID string `json:"DeviceID"`
	}

	if err := json.NewDecoder(strings.NewReader(string(body))).Decode(&discovery); err != nil {
		p.logger.Warn("⚠️  Failed to parse discovery JSON, using default device ID",
			logger.ErrorField("error", err))
		return nil // Don't fail if we can't parse, just use default
	}

	if discovery.DeviceID != "" {
		p.deviceID = discovery.DeviceID
		p.logger.Debug("✅ Successfully updated device ID",
			logger.String("device_id", p.deviceID))
	}

	return nil
}

// setupProxyRequest creates a proxy request with proper URL and headers.
func (p *HDHRProxy) setupProxyRequest(r *http.Request) (*RequestSetup, error) {
	// Create target URL
	targetURL := &url.URL{
		Scheme:   "http",
		Host:     p.HDHRIP,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
	}

	// Create proxy request
	proxyReq, err := http.NewRequest(r.Method, targetURL.String(), r.Body)
	if err != nil {
		return nil, utils.LogAndWrapError(err, "Error creating proxy request for %s", targetURL.String())
	}

	// Copy request headers efficiently
	copyHeaders(r.Header, proxyReq.Header)

	return &RequestSetup{
		TargetURL: targetURL,
		ProxyReq:  proxyReq,
	}, nil
}

// copyHeaders efficiently copies HTTP headers from source to destination.
func copyHeaders(src, dst http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

// executeProxyRequest sends the proxy request and handles the response.
func (p *HDHRProxy) executeProxyRequest(w http.ResponseWriter, setup *RequestSetup, originalReq *http.Request) error {
	// Send the request to the HDHomeRun
	resp, err := p.Client.Do(setup.ProxyReq)
	if err != nil {
		return utils.LogAndReturnWithHTTPError(w, http.StatusBadGateway, err,
			"Error forwarding request to %s", "Error forwarding request", setup.TargetURL.String())
	}
	defer resp.Body.Close()

	// Stream the response efficiently
	return p.streamResponse(w, resp, originalReq)
}

// HandleAppRequest processes app server requests by proxying to HDHomeRun and transforming responses.
func (p *HDHRProxy) HandleAppRequest(w http.ResponseWriter, r *http.Request) {
	setup, err := p.setupProxyRequest(r)
	if err != nil {
		http.Error(w, "Error creating proxy request", http.StatusInternalServerError)
		return
	}

	err = p.executeProxyRequest(w, setup, r)
	if err != nil {
		p.logger.Error("❌ Error in HandleAppRequest", logger.ErrorField("error", err))
		// Headers already sent, can't change status code
	}
}

// transformResponseBody modifies the response body content from the HDHomeRun device
// to ensure compatibility with media servers and clients. It performs several transformations:
// 1. Replaces the original device ID with the reversed version for client compatibility.
// 2. Updates URLs to point to the proxy server instead of directly to the HDHomeRun.
// 3. Adjusts port numbers and host information to maintain proper routing.
//
// Parameters:
//   - body: The original response body from the HDHomeRun.
//   - host: The host header from the original request (used for URL rewriting).
//
// Returns the transformed response body as a byte slice.
func (p *HDHRProxy) transformResponseBody(body []byte, host string) []byte {
	content := string(body)

	// Pre-calculate host parts to avoid repeated parsing
	hostParts := strings.Split(host, ":")
	hostName := hostParts[0]
	hostPort := "80"
	if len(hostParts) > 1 {
		hostPort = hostParts[1]
	}
	hostWithPort := host
	if hostPort == "80" {
		hostWithPort = hostName
	}

	// Pre-calculate replacement strings to avoid repeated concatenation
	reversedDeviceID := p.ReverseDeviceID()
	hdhrIPWithPort := p.HDHRIP + ":5004"
	hostNameWithPort := hostName + ":5004"

	// Use strings.Builder for efficient string building
	var result strings.Builder
	result.Grow(len(content) + 256) // Pre-allocate with some extra space for expansions

	// Process the content in a single pass with multiple replacements
	// This is more efficient than multiple separate ReplaceAll calls
	i := 0
	for i < len(content) {
		// Check for device ID replacement
		if i <= len(content)-len(p.DeviceID()) && content[i:i+len(p.DeviceID())] == p.DeviceID() {
			result.WriteString(reversedDeviceID)
			i += len(p.DeviceID())
			continue
		}

		// Check for HDHomeRun IP with port 5004 replacement
		if i <= len(content)-len(hdhrIPWithPort) && content[i:i+len(hdhrIPWithPort)] == hdhrIPWithPort {
			result.WriteString(hostNameWithPort)
			i += len(hdhrIPWithPort)
			continue
		}

		// Check for HDHomeRun IP replacement (be careful not to replace already processed URLs)
		if i <= len(content)-len(p.HDHRIP) && content[i:i+len(p.HDHRIP)] == p.HDHRIP {
			// Look ahead to see if this is followed by ":5004" (already handled above)
			if i+len(p.HDHRIP) < len(content) && content[i+len(p.HDHRIP):i+len(p.HDHRIP)+1] == ":" {
				// This might be the IP:port pattern, let it be handled by other cases
				result.WriteByte(content[i])
				i++
				continue
			}
			result.WriteString(hostWithPort)
			i += len(p.HDHRIP)
			continue
		}

		// Check for AC4 replacement
		if i <= len(content)-3 && content[i:i+3] == "AC4" {
			result.WriteString("AC3")
			i += 3
			continue
		}

		// No replacement needed, copy the character
		result.WriteByte(content[i])
		i++
	}

	return []byte(result.String())
}

// APIHandler returns a http.Handler for the API endpoints.
func (p *HDHRProxy) APIHandler() http.Handler {
	mux := http.NewServeMux()

	// Handle all API requests
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p.ProxyRequest(w, r)
	})

	return mux
}

// ProxyRequest handles proxying a single HTTP request to the HDHomeRun
// and transforms the response appropriately.
func (p *HDHRProxy) ProxyRequest(w http.ResponseWriter, r *http.Request) {
	p.logger.Debug("🔄 Proxying request",
		logger.String("method", r.Method),
		logger.String("path", r.URL.Path))

	setup, err := p.setupProxyRequest(r)
	if err != nil {
		http.Error(w, "Error creating proxy request", http.StatusInternalServerError)
		return
	}

	p.logger.Debug("🎯 Target URL set",
		logger.String("target_url", setup.TargetURL.String()))
	p.logger.Debug("📡 Sending request to HDHomeRun")

	err = p.executeProxyRequest(w, setup, r)
	if err != nil {
		p.logger.Error("❌ Error streaming response", logger.ErrorField("error", err))
		// At this point headers are already sent, so we can't send a different HTTP error
		return
	}

	p.logger.Debug("✅ Successfully streamed response")
}

// streamWithLimitedTransformation streams large responses with basic transformations.
func (p *HDHRProxy) streamWithLimitedTransformation(w io.Writer, r io.Reader, host string) error {
	// For large responses, we'll do basic streaming with line-by-line processing
	// This is less efficient but prevents memory issues with very large responses

	// Pre-compile replacements for better performance
	replacer := strings.NewReplacer(
		p.DeviceID(), p.ReverseDeviceID(),
		p.HDHRIP+":5004", strings.Split(host, ":")[0]+":5004",
		"AC4", "AC3",
	)

	scanner := bufio.NewScanner(r)
	// Use a reasonable buffer size - 8KB is typically sufficient for API responses
	buf := make([]byte, 8*1024)   // 8KB buffer (reduced from 64KB)
	scanner.Buffer(buf, 256*1024) // 256KB max token size (reduced from 1MB)

	for scanner.Scan() {
		line := scanner.Text()

		// Apply all transformations at once using the replacer
		line = replacer.Replace(line)

		// Write the line back with newline
		if _, err := w.Write([]byte(line + "\n")); err != nil {
			return err
		}
	}

	return scanner.Err()
}

// streamResponse efficiently streams the response, transforming only when necessary.
func (p *HDHRProxy) streamResponse(w http.ResponseWriter, resp *http.Response, r *http.Request) error {
	// Copy response headers efficiently, except Content-Length (we may modify content)
	for key, values := range resp.Header {
		if strings.ToLower(key) != "content-length" {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
	}

	// Set response status code
	w.WriteHeader(resp.StatusCode)

	// Check if we need to transform the response
	contentType := resp.Header.Get("Content-Type")
	needsTransformation := p.needsTransformation(contentType)

	if !needsTransformation {
		// Stream binary or unknown content directly without transformation
		p.logger.Debug("📺 Streaming response directly",
			logger.String("content_type", contentType))
		_, err := io.Copy(w, resp.Body)
		return err
	}

	// For content that needs transformation, check the size
	contentLength := p.getContentLength(resp.Header)

	// If content is small (< 1MB) or size unknown, load and transform
	if contentLength == -1 || contentLength < maxInMemorySize {
		return p.transformSmallResponse(w, resp.Body, r.Host, contentLength)
	}

	// For large responses that need transformation, we'll stream with limited transformation
	// This is a fallback - in practice, HDHomeRun API responses are typically small
	p.logger.Debug("📦 Streaming large response with limited transformation")
	return p.streamWithLimitedTransformation(w, resp.Body, r.Host)
}

// needsTransformation checks if the content type requires transformation.
func (p *HDHRProxy) needsTransformation(contentType string) bool {
	return strings.Contains(contentType, "application/json") ||
		strings.Contains(contentType, "text/html") ||
		strings.Contains(contentType, "text/plain") ||
		strings.Contains(contentType, "text/xml")
}

// getContentLength extracts content length from headers.
func (p *HDHRProxy) getContentLength(headers http.Header) int64 {
	contentLengthStr := headers.Get("Content-Length")
	if contentLengthStr == "" {
		return -1
	}

	if cl, err := strconv.ParseInt(contentLengthStr, 10, 64); err == nil {
		return cl
	}
	return -1
}

// transformSmallResponse handles transformation of small responses using buffer pool.
func (p *HDHRProxy) transformSmallResponse(w http.ResponseWriter, body io.Reader, host string, contentLength int64) error {
	p.logger.Debug("💾 Loading response into memory for transformation",
		logger.Int64("size_bytes", contentLength))

	// Copy with a reasonable limit to prevent memory exhaustion
	limitedReader := io.LimitReader(body, maxInMemorySize)

	// Read directly into memory - for small responses, direct allocation is more efficient
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return err
	}

	// Transform the response
	transformed := p.transformResponseBody(data, host)

	// Write the transformed response
	_, err = w.Write(transformed)
	return err
}
