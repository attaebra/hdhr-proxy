package media

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/attaebra/hdhr-proxy/internal/logger"
)

// Mock HTTP server to simulate HDHomeRun
type mockHDHR struct {
	server *httptest.Server
}

func newMockHDHR() *mockHDHR {
	mock := &mockHDHR{}

	// Create a test server that simulates the HDHomeRun
	handler := http.NewServeMux()

	// Simulate a channel stream
	handler.HandleFunc("/auto/v", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp2t")

		// Send a simple test pattern (not real MPEG-TS, just for testing)
		for i := 0; i < 10; i++ {
			w.Write([]byte("TEST-PATTERN-DATA"))
			time.Sleep(50 * time.Millisecond) // Simulate stream delay
		}
	})

	// Simulate the device info endpoint
	handler.HandleFunc("/discover.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"DeviceID":"ABCDEF01","LineupURL":"/lineup.json"}`))
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

// TestNewTranscoder tests the creation of a new transcoder
func TestNewTranscoder(t *testing.T) {
	// Initialize logger for tests
	logger.SetLevel(logger.LevelDebug)

	transcoder := NewTranscoder("/path/to/ffmpeg", "192.168.1.100")

	if transcoder.FFmpegPath != "/path/to/ffmpeg" {
		t.Errorf("Expected FFmpegPath to be /path/to/ffmpeg, got %s", transcoder.FFmpegPath)
	}

	if transcoder.InputURL != "http://192.168.1.100:5004" {
		t.Errorf("Expected InputURL to be http://192.168.1.100:5004, got %s", transcoder.InputURL)
	}

	if transcoder.activeStreams == nil {
		t.Error("Expected activeStreams to be initialized")
	}
}

// TestCreateMediaHandler tests the creation of the HTTP handler
func TestCreateMediaHandler(t *testing.T) {
	// Initialize logger for tests
	logger.SetLevel(logger.LevelDebug)

	transcoder := NewTranscoder("/path/to/ffmpeg", "192.168.1.100")
	handler := transcoder.CreateMediaHandler()

	if handler == nil {
		t.Error("Expected handler to be non-nil")
	}

	// Test with invalid path
	req := httptest.NewRequest("GET", "/invalid/path", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Errorf("Expected status code 404 for invalid path, got %d", recorder.Code)
	}

	// Test status endpoint
	req = httptest.NewRequest("GET", "/status", nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status code 200 for status endpoint, got %d", recorder.Code)
	}

	if !strings.Contains(recorder.Body.String(), "HDHomeRun AC4 Proxy Status") {
		t.Error("Expected status page to contain title")
	}
}

// TestStopAllTranscoding tests the StopAllTranscoding method
func TestStopAllTranscoding(t *testing.T) {
	// Initialize logger for tests
	logger.SetLevel(logger.LevelDebug)

	transcoder := NewTranscoder("/path/to/ffmpeg", "192.168.1.100")

	// Add fake active stream
	transcoder.mutex.Lock()
	transcoder.activeStreams["5.1"] = time.Now()
	transcoder.mutex.Unlock()

	// Stop all transcoding
	transcoder.StopAllTranscoding()

	// Check if active streams were cleared
	transcoder.mutex.Lock()
	count := len(transcoder.activeStreams)
	transcoder.mutex.Unlock()

	if count != 0 {
		t.Errorf("Expected 0 active streams after StopAllTranscoding, got %d", count)
	}
}

// MockResponseWriter is a mock http.ResponseWriter for testing
type MockResponseWriter struct {
	headers http.Header
	body    bytes.Buffer
	status  int
}

func NewMockResponseWriter() *MockResponseWriter {
	return &MockResponseWriter{
		headers: make(http.Header),
		status:  http.StatusOK,
	}
}

func (m *MockResponseWriter) Header() http.Header {
	return m.headers
}

func (m *MockResponseWriter) Write(b []byte) (int, error) {
	return m.body.Write(b)
}

func (m *MockResponseWriter) WriteHeader(statusCode int) {
	m.status = statusCode
}

// TestTranscodeChannelNoFFmpeg tests that TranscodeChannel returns an error when ffmpeg is not found
func TestTranscodeChannelNoFFmpeg(t *testing.T) {
	// Skip if running in CI environment
	if os.Getenv("CI") == "true" {
		t.Skip("Skipping test in CI environment")
	}

	// Initialize logger for tests
	logger.SetLevel(logger.LevelDebug)

	// Use a non-existent ffmpeg path
	transcoder := NewTranscoder("/path/to/nonexistent/ffmpeg", "192.168.1.100")

	// Mock an http response writer
	w := NewMockResponseWriter()
	
	// Create a mock HTTP request
	req, err := http.NewRequest("GET", "/auto/v5.1", nil)
	if err != nil {
		t.Fatalf("Failed to create mock request: %v", err)
	}

	// Use a mock HDHomeRun
	mockHdhr := newMockHDHR()
	defer mockHdhr.Close()

	// Update transcoder to use mock server
	transcoder.InputURL = mockHdhr.URL()

	// This should fail because the mock server returns 404 for the specific URL pattern
	err = transcoder.TranscodeChannel(w, req, "5.1")

	// We expect an error
	if err == nil {
		t.Error("Expected error, but got nil")
	}

	// The error should be related to the HDHomeRun response or ffmpeg
	if err != nil && !strings.Contains(err.Error(), "response from HDHomeRun") &&
		!strings.Contains(err.Error(), "ffmpeg") {
		t.Errorf("Expected error related to HDHomeRun response or ffmpeg, got: %v", err)
	}
}

// TestFFmpegAvailability checks if ffmpeg is available for integration testing
func TestFFmpegAvailability(t *testing.T) {
	// This is more of an informational test to help with debugging
	// than an actual test of functionality
	if _, err := os.Stat("/usr/bin/ffmpeg"); os.IsNotExist(err) {
		t.Log("FFmpeg is NOT available at /usr/bin/ffmpeg")
	} else {
		t.Log("FFmpeg is available at /usr/bin/ffmpeg")
	}
}
