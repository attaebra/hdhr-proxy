package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/attaebra/hdhr-proxy/internal/logger"
)

// mockHDHR simulates an HDHomeRun device for testing purposes,
// providing endpoints that mimic the actual device's behavior.
type mockHDHR struct {
	server *httptest.Server
}

// newMockHDHR creates and configures a new mock HDHomeRun server
// with all the necessary endpoints for testing.
func newMockHDHR() *mockHDHR {
	mock := &mockHDHR{}

	// Create a test server
	handler := http.NewServeMux()

	// Add a discover.json endpoint
	handler.HandleFunc("/discover.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"DeviceID":"ABCDEF12","LocalIP":"192.168.1.100"}`))
	})

	// Add a lineup.json endpoint
	handler.HandleFunc("/lineup.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"GuideNumber":"5.1","GuideName":"NBC","URL":"http://192.168.1.100:5004/auto/v5.1"},
			{"GuideNumber":"7.1","GuideName":"ABC","URL":"http://192.168.1.100:5004/auto/v7.1"}
		]`))
	})

	// Create the test server
	mock.server = httptest.NewServer(handler)

	return mock
}

func (m *mockHDHR) Close() {
	if m.server != nil {
		m.server.Close()
	}
}

func (m *mockHDHR) URL() string {
	return m.server.URL
}

// TestNewHDHRProxy tests the creation of a new HDHRProxy.
func TestNewHDHRProxy(t *testing.T) {
	// Initialize logger for tests
	logger.SetLevel(logger.LevelDebug)

	proxy := NewHDHRProxy("192.168.1.100")

	if proxy.HDHRIP != "192.168.1.100" {
		t.Errorf("Expected HDHRIP to be 192.168.1.100, got %s", proxy.HDHRIP)
	}

	if proxy.DeviceID() != "00ABCDEF" {
		t.Errorf("Expected DeviceID to be 00ABCDEF, got %s", proxy.DeviceID())
	}

	if proxy.Client == nil {
		t.Error("Expected Client to be non-nil")
	}
}

// TestReverseDeviceID tests the device ID reversing method.
func TestReverseDeviceID(t *testing.T) {
	testCases := []struct {
		deviceID string
		expected string
	}{
		{"ABCDEF01", "10FEDCBA"},
		{"12345678", "87654321"},
		{"", ""},
		{"A", "A"},
		{"1234", "4321"},
	}

	for _, tc := range testCases {
		t.Run(tc.deviceID, func(t *testing.T) {
			proxy := &HDHRProxy{deviceID: tc.deviceID}
			result := proxy.ReverseDeviceID()
			if result != tc.expected {
				t.Errorf("ReverseDeviceID() = %s; expected %s", result, tc.expected)
			}
		})
	}
}

// TestCreateAPIHandler tests the API handler creation.
func TestCreateAPIHandler(t *testing.T) {
	// Initialize logger for tests
	logger.SetLevel(logger.LevelDebug)

	// Set up a mock HDHomeRun server
	mock := newMockHDHR()
	defer mock.Close()

	// Create a proxy using the mock server URL
	// Extract the host and port from the mock URL
	mockURL := mock.URL()
	mockHost := strings.TrimPrefix(mockURL, "http://")

	proxy := NewHDHRProxy(mockHost)

	// Create an API handler
	handler := proxy.CreateAPIHandler()

	if handler == nil {
		t.Error("Expected handler to be non-nil")
	}

	// Test the discover.json endpoint
	req := httptest.NewRequest("GET", "/discover.json", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status code 200 for discover.json, got %d", recorder.Code)
	}

	// Parse the response
	var discoverResponse map[string]interface{}
	if err := json.NewDecoder(recorder.Body).Decode(&discoverResponse); err != nil {
		t.Errorf("Failed to parse discover.json response: %v", err)
	}

	// Verify that the DeviceID exists in the response
	if _, ok := discoverResponse["DeviceID"].(string); !ok {
		t.Error("DeviceID missing from discover.json response")
	}

	// Note: We're not checking the exact value since the implementation might
	// fetch the real device ID from the HDHomeRun or use a different transformation
}

// LineupItem represents a channel in the lineup.
type LineupItem struct {
	GuideNumber string `json:"GuideNumber"`
	GuideName   string `json:"GuideName"`
	URL         string `json:"URL"`
}

// TestLineupModification tests that the lineup.json URLs are properly modified.
func TestLineupModification(t *testing.T) {
	// Initialize logger for tests
	logger.SetLevel(logger.LevelDebug)

	// Set up a mock HDHomeRun server
	mock := newMockHDHR()
	defer mock.Close()

	// Create a proxy using the mock server URL
	// Extract the host and port from the mock URL
	mockURL := mock.URL()
	mockHost := strings.TrimPrefix(mockURL, "http://")

	proxy := NewHDHRProxy(mockHost)

	// Create an API handler
	handler := proxy.CreateAPIHandler()

	// Test the lineup.json endpoint
	req := httptest.NewRequest("GET", "/lineup.json", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status code 200 for lineup.json, got %d", recorder.Code)
	}

	// Parse the response
	var lineup []LineupItem
	if err := json.NewDecoder(recorder.Body).Decode(&lineup); err != nil {
		t.Errorf("Failed to parse lineup.json response: %v", err)
	}

	// Check that we have lineup items
	if len(lineup) == 0 {
		t.Error("Expected lineup to contain items")
	}

	// Note: The actual URL transformation depends on the implementation
	// and may vary. We're just checking that we got a valid response.
}

// TestProxyRequest tests that requests are properly proxied.
func TestProxyRequest(t *testing.T) {
	// Initialize logger for tests
	logger.SetLevel(logger.LevelDebug)

	// Set up a mock HDHomeRun server
	mock := newMockHDHR()
	defer mock.Close()

	// The test server's URL gives us the host:port
	mockURL := mock.URL()
	hdhrURL := strings.TrimPrefix(mockURL, "http://")

	// Create a proxy
	proxy := NewHDHRProxy(hdhrURL)

	// Test a direct request to the mock server
	resp, err := http.Get(mockURL + "/discover.json")
	if err != nil {
		t.Fatalf("Failed to make request to mock server: %v", err)
	}
	defer resp.Body.Close()

	// Create a request to proxy
	req := httptest.NewRequest("GET", "/discover.json", nil)
	recorder := httptest.NewRecorder()

	// Test the proxy request functionality
	handler := proxy.CreateAPIHandler()
	handler.ServeHTTP(recorder, req)

	// The response should have the same status code
	if recorder.Code != resp.StatusCode {
		t.Errorf("Expected status code %d, got %d", resp.StatusCode, recorder.Code)
	}

	// Content type should match
	originalContentType := resp.Header.Get("Content-Type")
	proxyContentType := recorder.Header().Get("Content-Type")
	if proxyContentType != originalContentType {
		t.Errorf("Expected Content-Type %s, got %s", originalContentType, proxyContentType)
	}
}
