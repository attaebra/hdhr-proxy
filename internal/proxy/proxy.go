// Package proxy provides functionality to proxy and transform requests to/from
// the HDHomeRun device. It handles request forwarding, response modification,
// and maintains compatibility with media streaming clients.
package proxy

import (
	"bufio"
	"bytes"
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

// HDHRProxy handles proxying requests to the HDHomeRun and transforming the responses.
type HDHRProxy struct {
	HDHRIP   string
	deviceID string
	Client   interfaces.HTTPClient
}

// Ensure HDHRProxy implements the HDHRProxy interface.
var _ interfaces.HDHRProxy = (*HDHRProxy)(nil)

// NewHDHRProxy creates a new HDHomeRun proxy instance.
// hdhrIP is the IP address of the HDHomeRun device to proxy requests to.
// Returns a configured proxy instance with the device ID fetched from the HDHomeRun.
func NewHDHRProxy(hdhrIP string) *HDHRProxy {
	// Create an optimized HTTP client with reasonable timeout for API requests
	optimizedClient := utils.HTTPClient(30 * time.Second)

	return &HDHRProxy{
		HDHRIP:   hdhrIP,
		deviceID: "00ABCDEF", // Default device ID, will be updated
		Client:   optimizedClient,
	}
}

// NewHDHRProxyWithDependencies creates a new HDHomeRun proxy instance with injected dependencies.
func NewHDHRProxyWithDependencies(hdhrIP string, httpClient interfaces.HTTPClient) interfaces.HDHRProxy {
	return &HDHRProxy{
		HDHRIP:   hdhrIP,
		deviceID: "00ABCDEF", // Default device ID, will be updated
		Client:   httpClient,
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
	logger.Debug("Fetching device ID from HDHomeRun at %s", p.HDHRIP)
	resp, err := p.Client.Get("http://" + p.HDHRIP + "/discover.json")
	if err != nil {
		return utils.LogAndWrapError(err, "failed to connect to HDHomeRun at %s", p.HDHRIP)
	}
	defer resp.Body.Close()

	logger.Debug("Received response with status: %d", resp.StatusCode)

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
		logger.Warn("Failed to parse discovery JSON, using default device ID: %v", err)
		return nil // Don't fail if we can't parse, just use default
	}

	if discovery.DeviceID != "" {
		p.deviceID = discovery.DeviceID
		logger.Debug("Successfully updated device ID to: %s", p.deviceID)
	}

	return nil
}

// HandleAppRequest processes app server requests by proxying to HDHomeRun and transforming responses.
func (p *HDHRProxy) HandleAppRequest(w http.ResponseWriter, r *http.Request) {
	// Create a new URL from the original request
	targetURL := &url.URL{
		Scheme:   "http",
		Host:     p.HDHRIP,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
	}

	// Create a new request
	proxyReq, err := http.NewRequest(r.Method, targetURL.String(), r.Body)
	if err != nil {
		http.Error(w, "Error creating proxy request", http.StatusInternalServerError)
		return
	}

	// Copy request headers
	for key, values := range r.Header {
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}

	// Send the request to the HDHomeRun
	resp, err := p.Client.Do(proxyReq)
	if err != nil {
		http.Error(w, "Error forwarding request", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Stream the response efficiently
	err = p.streamResponse(w, resp, r)
	if err != nil {
		logger.Error("Error streaming response in HandleAppRequest: %v", err)
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

// CreateAPIHandler returns a http.Handler for the API endpoints.
func (p *HDHRProxy) CreateAPIHandler() http.Handler {
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
	logger.Debug("Proxying request: %s %s", r.Method, r.URL.Path)

	// Create a new URL from the original request
	targetURL := &url.URL{
		Scheme:   "http",
		Host:     p.HDHRIP,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
	}

	logger.Debug("Target URL: %s", targetURL.String())

	// Create a new request
	proxyReq, err := http.NewRequest(r.Method, targetURL.String(), r.Body)
	if err != nil {
		logger.Error("Error creating proxy request: %v", err)
		http.Error(w, "Error creating proxy request", http.StatusInternalServerError)
		return
	}

	// Copy request headers
	for key, values := range r.Header {
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}

	// Send the request to the HDHomeRun
	logger.Debug("Sending request to HDHomeRun")
	resp, err := p.Client.Do(proxyReq)
	if err != nil {
		logger.Error("Error forwarding request: %v", err)
		http.Error(w, "Error forwarding request", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	logger.Debug("Received response with status: %d", resp.StatusCode)

	// Stream the response efficiently instead of loading everything into memory
	err = p.streamResponse(w, resp, r)
	if err != nil {
		logger.Error("Error streaming response: %v", err)
		// At this point headers are already sent, so we can't send a different HTTP error
	}

	logger.Debug("Successfully streamed response")
}

// streamResponse efficiently streams the response, transforming only when necessary.
func (p *HDHRProxy) streamResponse(w http.ResponseWriter, resp *http.Response, r *http.Request) error {
	// Copy response headers, except Content-Length (we may modify content)
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
	needsTransformation := strings.Contains(contentType, "application/json") ||
		strings.Contains(contentType, "text/html") ||
		strings.Contains(contentType, "text/plain") ||
		strings.Contains(contentType, "text/xml")

	if !needsTransformation {
		// Stream binary or unknown content directly without transformation
		logger.Debug("Streaming response directly (Content-Type: %s)", contentType)
		_, err := io.Copy(w, resp.Body)
		return err
	}

	// For content that needs transformation, check the size
	contentLengthStr := resp.Header.Get("Content-Length")
	var contentLength int64 = -1
	if contentLengthStr != "" {
		if cl, err := strconv.ParseInt(contentLengthStr, 10, 64); err == nil {
			contentLength = cl
		}
	}

	// If content is small (< 1MB) or size unknown, load and transform
	const maxInMemorySize = 1024 * 1024 // 1MB
	if contentLength == -1 || contentLength < maxInMemorySize {
		logger.Debug("Loading response into memory for transformation (size: %d bytes)", contentLength)

		// Use a buffer with reasonable initial capacity
		var buf bytes.Buffer
		if contentLength > 0 {
			buf.Grow(int(contentLength))
		} else {
			buf.Grow(4096) // Default 4KB for unknown size
		}

		// Copy with a reasonable limit to prevent memory exhaustion
		limitedReader := io.LimitReader(resp.Body, maxInMemorySize)
		_, err := io.Copy(&buf, limitedReader)
		if err != nil {
			return err
		}

		// Transform the response
		transformed := p.transformResponseBody(buf.Bytes(), r.Host)

		// Write the transformed response
		_, err = w.Write(transformed)
		return err
	}

	// For large responses that need transformation, we'll stream with limited transformation
	// This is a fallback - in practice, HDHomeRun API responses are typically small
	logger.Debug("Streaming large response with limited transformation")
	return p.streamWithLimitedTransformation(w, resp.Body, r.Host)
}

// streamWithLimitedTransformation streams large responses with basic transformations.
func (p *HDHRProxy) streamWithLimitedTransformation(w io.Writer, r io.Reader, host string) error {
	// For large responses, we'll do basic streaming with line-by-line processing
	// This is less efficient but prevents memory issues with very large responses

	scanner := bufio.NewScanner(r)
	// Increase buffer size for larger lines
	buf := make([]byte, 64*1024)   // 64KB buffer
	scanner.Buffer(buf, 1024*1024) // 1MB max token size

	for scanner.Scan() {
		line := scanner.Text()

		// Apply basic transformations per line
		line = strings.ReplaceAll(line, p.DeviceID(), p.ReverseDeviceID())
		line = strings.ReplaceAll(line, p.HDHRIP+":5004", strings.Split(host, ":")[0]+":5004")
		line = strings.ReplaceAll(line, "AC4", "AC3")

		// Write the line back with newline
		if _, err := w.Write([]byte(line + "\n")); err != nil {
			return err
		}
	}

	return scanner.Err()
}
