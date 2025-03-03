package proxy

import (
	"io"
	"net/http"
	"net/url"
	"strings"
)

// HDHRProxy handles proxying requests to the HDHomeRun and transforming the responses
type HDHRProxy struct {
	HDHRIP   string
	DeviceID string
	Client   *http.Client
}

// NewHDHRProxy creates a new HDHomeRun proxy instance
func NewHDHRProxy(hdhrIP string) *HDHRProxy {
	return &HDHRProxy{
		HDHRIP:   hdhrIP,
		DeviceID: "00ABCDEF", // Default device ID, will be updated
		Client:   &http.Client{},
	}
}

// ReverseDeviceID reverses the device ID string
func (p *HDHRProxy) ReverseDeviceID() string {
	runes := []rune(p.DeviceID)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// FetchDeviceID retrieves the actual device ID from the HDHomeRun
func (p *HDHRProxy) FetchDeviceID() error {
	resp, err := p.Client.Get("http://" + p.HDHRIP + "/discover.json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read and parse the response to extract DeviceID
	// TODO: Implement JSON parsing to extract the DeviceID

	return nil
}

// HandleAppRequest processes app server requests by proxying to HDHomeRun and transforming responses
func (p *HDHRProxy) HandleAppRequest(w http.ResponseWriter, r *http.Request) {
	// Create a new URL from the original request
	targetURL := &url.URL{
		Scheme: "http",
		Host:   p.HDHRIP,
		Path:   r.URL.Path,
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

	// Copy response headers, except Content-Length
	for key, values := range resp.Header {
		if strings.ToLower(key) != "content-length" {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
	}

	// Set response status code
	w.WriteHeader(resp.StatusCode)

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Error reading response body", http.StatusInternalServerError)
		return
	}

	// Transform the response
	body = p.transformResponseBody(body, r.Host)

	// Write the transformed body to the response
	_, err = w.Write(body)
	if err != nil {
		// Log error but can't do much at this point as headers are already sent
		// TODO: Add proper logging
	}
}

// transformResponseBody modifies the response body content
func (p *HDHRProxy) transformResponseBody(body []byte, host string) []byte {
	content := string(body)

	// Get host and port
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

	// Replace device ID
	content = strings.ReplaceAll(content, p.DeviceID, p.ReverseDeviceID())

	// Replace HDHomeRun IP references
	// First, replace IPs with port 5004
	content = strings.ReplaceAll(content, p.HDHRIP+":5004", hostName+":5004")
	
	// Then replace the regular IP (without port)
	// We need to handle this carefully to avoid affecting already replaced URLs
	// Split and join to avoid modifying the already replaced URLs
	parts := strings.Split(content, ":")
	for i := 0; i < len(parts); i++ {
		if i > 0 && strings.HasSuffix(parts[i-1], p.HDHRIP) {
			// Skip as this is part of a URL we already handled
			continue
		}
		parts[i] = strings.ReplaceAll(parts[i], p.HDHRIP, hostWithPort)
	}
	content = strings.Join(parts, ":")

	// Replace AC4 with AC3
	content = strings.ReplaceAll(content, "AC4", "AC3")

	return []byte(content)
} 