package media

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/attaebra/hdhr-proxy/internal/logger"
	"github.com/attaebra/hdhr-proxy/internal/proxy"
)

// Transcoder manages the FFmpeg process for transcoding AC4 to EAC3
type Transcoder struct {
	FFmpegPath     string
	InputURL       string
	ctx            context.Context
	cancel         context.CancelFunc
	cmd            *exec.Cmd
	mutex          sync.Mutex
	activeStreams  map[string]time.Time // Track active streams by channel ID
	RequestTimeout time.Duration        // HTTP request timeout
	proxy          *proxy.HDHRProxy     // Reference to the proxy for API access
	ac4Channels    map[string]bool      // Track which channels have AC4 audio
}

// NewTranscoder creates a new transcoder instance
func NewTranscoder(ffmpegPath, hdhrIP string) *Transcoder {
	ctx, cancel := context.WithCancel(context.Background())

	// Default to no timeout (0)
	var requestTimeout time.Duration

	// Only set a timeout if explicitly configured
	if timeoutStr := os.Getenv("REQUEST_TIMEOUT"); timeoutStr != "" {
		if parsedTimeout, err := time.ParseDuration(timeoutStr); err == nil {
			requestTimeout = parsedTimeout
			logger.Debug("Using custom request timeout: %s", requestTimeout)
		} else {
			logger.Warn("Invalid REQUEST_TIMEOUT format, using no timeout")
		}
	} else {
		logger.Debug("No timeout configured, streaming will continue indefinitely")
	}

	// Create proxy for API access
	p := proxy.NewHDHRProxy(hdhrIP)

	t := &Transcoder{
		FFmpegPath:     ffmpegPath,
		InputURL:       fmt.Sprintf("http://%s:5004", hdhrIP),
		ctx:            ctx,
		cancel:         cancel,
		activeStreams:  make(map[string]time.Time),
		RequestTimeout: requestTimeout,
		proxy:          p,
		ac4Channels:    make(map[string]bool),
	}

	// Initialize AC4 channel list
	if err := t.fetchAC4Channels(); err != nil {
		logger.Warn("Failed to fetch AC4 channels: %v", err)
	}

	return t
}

// fetchAC4Channels fetches the lineup from the HDHomeRun and identifies channels with AC4 audio
func (t *Transcoder) fetchAC4Channels() error {
	// Create an HTTP client
	client := &http.Client{
		Timeout: 5 * time.Second, // Add a reasonable timeout for API requests
	}

	// Create the request
	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/lineup.json", t.proxy.HDHRIP), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	logger.Debug("Fetching lineup from %s", t.proxy.HDHRIP)

	// Execute the request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch lineup: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("invalid response from HDHomeRun: %d", resp.StatusCode)
	}

	// Parse the response body
	var lineup []struct {
		GuideNumber string `json:"GuideNumber"`
		GuideName   string `json:"GuideName"`
		URL         string `json:"URL"`
		HD          int    `json:"HD"`
		Favorite    int    `json:"Favorite"`
		AudioCodec  string `json:"AudioCodec"` // Audio codec field
		VideoCodec  string `json:"VideoCodec"` // Video codec field
	}

	if err := json.NewDecoder(resp.Body).Decode(&lineup); err != nil {
		return fmt.Errorf("failed to parse lineup: %w", err)
	}

	ac4Count := 0
	// Check for AC4 audio codec
	for _, channel := range lineup {
		// Use AudioCodec field to directly identify AC4 channels
		hasAC4 := strings.ToUpper(channel.AudioCodec) == "AC4"

		t.ac4Channels[channel.GuideNumber] = hasAC4

		if hasAC4 {
			ac4Count++
			logger.Info("Identified AC4 audio channel: %s - %s (Audio: %s, Video: %s)",
				channel.GuideNumber, channel.GuideName, channel.AudioCodec, channel.VideoCodec)
		} else {
			logger.Debug("Regular channel: %s - %s (Audio: %s, Video: %s)",
				channel.GuideNumber, channel.GuideName,
				getDefaultString(channel.AudioCodec, "Unknown"),
				getDefaultString(channel.VideoCodec, "Unknown"))
		}
	}

	logger.Info("Found %d channels with AC4 audio out of %d total channels",
		ac4Count, len(lineup))

	return nil
}

// getDefaultString returns the default value if the input is empty
func getDefaultString(input, defaultVal string) string {
	if input == "" {
		return defaultVal
	}
	return input
}

// isAC4Channel checks if a channel uses AC4 audio codec
func (t *Transcoder) isAC4Channel(channel string) bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	isAC4, exists := t.ac4Channels[channel]
	if !exists {
		// If we don't know, assume it might have AC4 to be safe
		logger.Debug("Unknown channel %s, assuming it may have AC4 audio", channel)
		return true
	}
	return isAC4
}

// isClientDisconnectError determines if an error is due to client disconnection
func isClientDisconnectError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "connection reset by peer") ||
		strings.Contains(errStr, "client disconnected") ||
		strings.Contains(errStr, "i/o timeout") ||
		strings.Contains(errStr, "use of closed network connection")
}

// DirectStreamChannel streams the channel directly without transcoding
func (t *Transcoder) DirectStreamChannel(w http.ResponseWriter, r *http.Request, channel string) error {
	start := time.Now()

	// Track this stream in our active streams
	t.mutex.Lock()
	t.activeStreams[channel] = start
	activeCount := len(t.activeStreams)
	t.mutex.Unlock()

	logger.Info("Direct streaming (no transcode) for channel: %s (active streams: %d)", channel, activeCount)
	logger.Debug("Using input URL: %s/auto/v%s", t.InputURL, channel)

	// Create a context that will be canceled when the client disconnects
	ctx, cancel := context.WithCancel(context.Background())

	// Monitor for client disconnection
	go func() {
		<-r.Context().Done()
		logger.Debug("Detected client disconnect for channel %s - canceling direct stream", channel)
		cancel()
	}()

	defer func() {
		if r := recover(); r != nil {
			logger.Error("Recovered from panic in DirectStreamChannel: %v\nStack: %s", r, debug.Stack())
		}

		// Cancel the context to release resources
		cancel()

		// Remove this stream from active streams
		t.mutex.Lock()
		delete(t.activeStreams, channel)
		duration := time.Since(start).Seconds()
		t.mutex.Unlock()

		logger.Info("Direct streaming session for channel %s ended after %.2f seconds", channel, duration)
	}()

	// Create an HTTP client to fetch the stream
	client := &http.Client{}

	// Only set timeout if greater than 0
	if t.RequestTimeout > 0 {
		client.Timeout = t.RequestTimeout
		logger.Debug("Using HTTP client timeout: %s", t.RequestTimeout)
	} else {
		logger.Debug("No timeout set for HTTP client, stream will continue until closed")
	}

	// Create the request with context
	sourceURL := fmt.Sprintf("%s/auto/v%s", t.InputURL, channel)
	logger.Debug("Connecting to source URL: %s", sourceURL)
	req, err := http.NewRequestWithContext(ctx, "GET", sourceURL, nil)
	if err != nil {
		logger.Error("Failed to create HTTP request: %v", err)
		http.Error(w, "Failed to create HTTP request", http.StatusInternalServerError)
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add some default headers
	req.Header.Set("User-Agent", "hdhr-proxy/1.0")

	// Execute the request
	logger.Debug("Sending request to HDHomeRun...")
	connStart := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("Failed to fetch stream: %v", err)
		http.Error(w, "Failed to fetch stream from HDHomeRun", http.StatusBadGateway)
		return fmt.Errorf("failed to fetch stream: %w", err)
	}
	logger.Debug("Connected to HDHomeRun in %d ms", time.Since(connStart).Milliseconds())

	defer resp.Body.Close()

	// Check response status
	logger.Debug("Received response with status: %d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("Invalid response from HDHomeRun: %d", resp.StatusCode)
		logger.Error(errMsg)
		http.Error(w, errMsg, http.StatusBadGateway)
		return fmt.Errorf("%s", errMsg)
	}

	// Log content type and headers for debugging
	logger.Debug("Response content type: %s", resp.Header.Get("Content-Type"))
	logger.Debug("Response headers: %v", resp.Header)

	// Set appropriate headers for the response
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("X-Direct-Stream", "true")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")

	// Copy from HDHomeRun to response
	copied, err := io.Copy(w, resp.Body)
	if err != nil && err != io.EOF && ctx.Err() == nil {
		if isClientDisconnectError(err) {
			// This is normal when client disconnects, don't treat as error
			logger.Debug("Client disconnected during direct stream for channel %s: %v", channel, err)
		} else {
			logger.Error("Error copying from HDHomeRun to response: %v", err)
		}
		return fmt.Errorf("stream interrupted: %w", err)
	}

	logger.Debug("Finished direct stream copy, bytes copied: %d", copied)
	return nil
}

// TranscodeChannel starts the ffmpeg process to transcode from AC4 to EAC3
func (t *Transcoder) TranscodeChannel(w http.ResponseWriter, r *http.Request, channel string) error {
	start := time.Now()

	// Track this stream in our active streams
	t.mutex.Lock()
	t.activeStreams[channel] = start
	activeCount := len(t.activeStreams)
	t.mutex.Unlock()

	logger.Info("Starting transcoding for channel: %s (active streams: %d)", channel, activeCount)
	logger.Debug("Using input URL: %s/auto/v%s", t.InputURL, channel)

	// Create a context that will be canceled when the client disconnects
	ctx, cancel := context.WithCancel(context.Background())

	// Monitor for client disconnection
	go func() {
		<-r.Context().Done()
		logger.Debug("Detected client disconnect for channel %s - canceling transcoding", channel)
		cancel()
	}()

	defer func() {
		if r := recover(); r != nil {
			logger.Error("Recovered from panic in TranscodeChannel: %v\nStack: %s", r, debug.Stack())
		}

		// Cancel the context to release resources
		cancel()

		// Remove this stream from active streams
		t.mutex.Lock()
		delete(t.activeStreams, channel)
		duration := time.Since(start).Seconds()
		t.mutex.Unlock()

		logger.Info("Transcoding session for channel %s ended after %.2f seconds", channel, duration)
	}()

	// Create an HTTP client to fetch the stream
	client := &http.Client{}

	// Only set timeout if greater than 0
	if t.RequestTimeout > 0 {
		client.Timeout = t.RequestTimeout
		logger.Debug("Using HTTP client timeout: %s", t.RequestTimeout)
	} else {
		logger.Debug("No timeout set for HTTP client, stream will continue until closed")
	}

	// Create the request with context
	sourceURL := fmt.Sprintf("%s/auto/v%s", t.InputURL, channel)
	logger.Debug("Connecting to source URL: %s", sourceURL)
	req, err := http.NewRequestWithContext(ctx, "GET", sourceURL, nil)
	if err != nil {
		logger.Error("Failed to create HTTP request: %v", err)
		http.Error(w, "Failed to create HTTP request", http.StatusInternalServerError)
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add some default headers
	req.Header.Set("User-Agent", "hdhr-proxy/1.0")

	// Execute the request
	logger.Debug("Sending request to HDHomeRun...")
	connStart := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("Failed to fetch stream: %v", err)
		http.Error(w, "Failed to fetch stream", http.StatusBadGateway)
		return fmt.Errorf("failed to fetch stream: %w", err)
	}
	logger.Debug("Connected to HDHomeRun in %d ms", time.Since(connStart).Milliseconds())

	defer resp.Body.Close()

	// Check response status
	logger.Debug("Received response with status: %d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("Invalid response from HDHomeRun: %d", resp.StatusCode)
		logger.Error(errMsg)
		http.Error(w, errMsg, http.StatusBadGateway)
		return fmt.Errorf("%s", errMsg)
	}

	// Log content type and headers for debugging
	logger.Debug("Response content type: %s", resp.Header.Get("Content-Type"))
	logger.Debug("Response headers: %v", resp.Header)

	// Set up ffmpeg command with minimal options, matching the JavaScript implementation
	logger.Debug("Setting up ffmpeg command with path: %s", t.FFmpegPath)

	cmd := exec.CommandContext(ctx, t.FFmpegPath,
		"-nostats",
		"-hide_banner",
		"-loglevel", "warning",
		"-i", "pipe:",
		"-map", "0:v",
		"-map", "0:a",
		"-c:v", "copy",
		"-ar", "48000",
		"-c:a", "eac3",
		"-c:d", "copy",
		"-f", "mpegts",
		"-",
	)

	// Get pipes for stdin and stdout
	stdin, err := cmd.StdinPipe()
	if err != nil {
		logger.Error("Failed to get stdin pipe: %v", err)
		http.Error(w, "Failed to get stdin pipe", http.StatusInternalServerError)
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logger.Error("Failed to get stdout pipe: %v", err)
		http.Error(w, "Failed to get stdout pipe", http.StatusInternalServerError)
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		logger.Error("Failed to get stderr pipe: %v", err)
		http.Error(w, "Failed to get stderr pipe", http.StatusInternalServerError)
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	// Start the ffmpeg process
	logger.Debug("Starting ffmpeg process...")
	ffmpegStart := time.Now()

	err = cmd.Start()
	if err != nil {
		logger.Error("Failed to start ffmpeg: %v", err)
		http.Error(w, "Failed to start ffmpeg", http.StatusInternalServerError)
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	ffmpegPid := cmd.Process.Pid
	logger.Debug("ffmpeg process started successfully with PID: %d in %d ms",
		ffmpegPid, time.Since(ffmpegStart).Milliseconds())

	// Set up cleanup for when we're done
	defer func() {
		if cmd.Process != nil {
			logger.Debug("Killing ffmpeg process with PID: %d", ffmpegPid)
			if err := cmd.Process.Kill(); err != nil {
				logger.Error("Failed to kill ffmpeg process: %v", err)
			}
		}
	}()

	// Log stderr output from ffmpeg
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			logger.Debug("ffmpeg[%d]: %s", ffmpegPid, scanner.Text())
		}
	}()

	// Set appropriate headers for the response
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("X-Transcoded-By", "hdhr-proxy")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")

	// Create a WaitGroup to wait for both goroutines to finish
	var wg sync.WaitGroup
	wg.Add(2)

	// Copy from HDHomeRun to ffmpeg stdin
	go func() {
		defer wg.Done()
		defer stdin.Close()
		logger.Debug("Starting stream copy from HDHomeRun to ffmpeg for channel %s...", channel)

		copied, err := io.Copy(stdin, resp.Body)
		if err != nil && err != io.EOF && ctx.Err() == nil {
			if isClientDisconnectError(err) {
				logger.Debug("Client disconnected during HDHomeRun to ffmpeg copy for channel %s: %v", channel, err)
			} else {
				logger.Error("Error copying from HDHomeRun to ffmpeg: %v", err)
			}
		}
		logger.Debug("Finished copying from HDHomeRun to ffmpeg, bytes copied: %d", copied)
	}()

	// Copy from ffmpeg stdout to response
	go func() {
		defer wg.Done()
		logger.Debug("Starting stream copy from ffmpeg to response for channel %s...", channel)

		copied, err := io.Copy(w, stdout)
		if err != nil && err != io.EOF && ctx.Err() == nil {
			if isClientDisconnectError(err) {
				logger.Debug("Client disconnected during ffmpeg to client copy for channel %s: %v", channel, err)
			} else {
				logger.Error("Error copying from ffmpeg to response: %v", err)
			}
		}
		logger.Debug("Finished copying from ffmpeg to response, bytes copied: %d", copied)
	}()

	// Wait for both copy operations to complete
	wg.Wait()

	// Wait for the process to exit
	err = cmd.Wait()
	if err != nil && ctx.Err() == nil {
		logger.Error("ffmpeg process exited with error: %v", err)
		// Don't send error to client at this point, response already started
	}

	logger.Debug("Transcoding completed for channel %s", channel)
	return nil
}

// Stop stops the transcoding process
func (t *Transcoder) Stop() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	logger.Info("Stopping transcoder gracefully")

	if t.cancel != nil {
		t.cancel()
	}

	// Log active streams that are being stopped
	for channel, startTime := range t.activeStreams {
		duration := time.Since(startTime).Seconds()
		logger.Info("Stopping active stream for channel %s (duration: %.2f seconds)",
			channel, duration)
	}

	// Clear active streams
	t.activeStreams = make(map[string]time.Time)
}

// CreateMediaHandler returns a http.Handler for the media endpoints
func (t *Transcoder) CreateMediaHandler() http.Handler {
	mux := http.NewServeMux()

	// Handle auto/v{channel} requests for channel transcoding
	mux.HandleFunc("/auto/", func(w http.ResponseWriter, r *http.Request) {
		remoteAddr := r.RemoteAddr
		userAgent := r.UserAgent()

		logger.Info("Received media request: %s %s from %s (User-Agent: %s)",
			r.Method, r.URL.Path, remoteAddr, userAgent)

		// Extract channel from URL path
		if !strings.HasPrefix(r.URL.Path, "/auto/v") {
			logger.Debug("Path %s doesn't match /auto/v pattern, returning 404", r.URL.Path)
			http.NotFound(w, r)
			return
		}

		channel := strings.TrimPrefix(r.URL.Path, "/auto/v")
		if channel == "" {
			logger.Warn("Empty channel requested from %s", remoteAddr)
			http.Error(w, "Missing channel number", http.StatusBadRequest)
			return
		}

		// Check if this channel has AC4 audio needing transcoding
		if t.isAC4Channel(channel) {
			logger.Info("Processing channel %s with AC4 audio - transcoding to EAC3", channel)
			if err := t.TranscodeChannel(w, r, channel); err != nil {
				logger.Error("Transcoding error for channel %s: %v", channel, err)
				// Error already sent to client by TranscodeChannel
			}
		} else {
			// For channels without AC4 audio, stream directly without transcoding
			logger.Info("Processing channel %s without AC4 audio - direct streaming", channel)
			if err := t.DirectStreamChannel(w, r, channel); err != nil {
				logger.Error("Direct streaming error for channel %s: %v", channel, err)
				// Error already handled by DirectStreamChannel
			}
		}

		logger.Debug("Media handler completed for channel: %s from client %s", channel, remoteAddr)
	})

	// Add a helper function to write output and log it at debug level
	writeOutput := func(w http.ResponseWriter, format string, args ...interface{}) {
		msg := fmt.Sprintf(format, args...)
		logger.Debug("Status output: %s", strings.TrimSpace(msg))
		fmt.Fprint(w, msg)
	}

	// Status endpoint handler
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		logger.Info("Status endpoint accessed")

		t.mutex.Lock()
		activeStreams := len(t.activeStreams)

		// Create a copy of the active streams data for display
		streams := make(map[string]float64)
		channelIsAC4 := make(map[string]bool)

		for channel, startTime := range t.activeStreams {
			streams[channel] = time.Since(startTime).Seconds()
			channelIsAC4[channel] = t.ac4Channels[channel]
		}

		// Count AC4 channels
		ac4Count := 0
		for _, isAC4 := range t.ac4Channels {
			if isAC4 {
				ac4Count++
			}
		}

		totalChannels := len(t.ac4Channels)
		t.mutex.Unlock()

		w.Header().Set("Content-Type", "text/plain")
		writeOutput(w, "HDHomeRun AC4 Proxy Status\n")
		writeOutput(w, "=========================\n")
		writeOutput(w, "Active Streams: %d\n", activeStreams)
		writeOutput(w, "Total Channels: %d\n", totalChannels)
		writeOutput(w, "AC4 Audio Channels: %d\n\n", ac4Count)

		if activeStreams > 0 {
			writeOutput(w, "Channel    Duration (s)  Transcoding\n")
			writeOutput(w, "-----------------------------------\n")
			for channel, duration := range streams {
				isAC4 := channelIsAC4[channel]
				transcoding := "No"
				if isAC4 {
					transcoding = "Yes (AC4→EAC3)"
				}
				writeOutput(w, "%-10s %-12.2f %s\n", channel, duration, transcoding)
			}
			writeOutput(w, "\n")
		}

		// Write system information
		writeOutput(w, "HDHomeRun Device: %s\n", t.proxy.HDHRIP)
		writeOutput(w, "FFmpeg Path: %s\n", t.FFmpegPath)
		if t.RequestTimeout > 0 {
			writeOutput(w, "Stream Timeout: %s\n", t.RequestTimeout)
		} else {
			writeOutput(w, "Stream Timeout: None (streams indefinitely)\n")
		}
	})

	return mux
}

// StopAllTranscoding stops any running transcoding processes
func (t *Transcoder) StopAllTranscoding() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	activeStreams := len(t.activeStreams)
	logger.Info("Stopping all transcoding processes (%d active streams)", activeStreams)

	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}

	if t.cmd != nil && t.cmd.Process != nil {
		pid := t.cmd.Process.Pid
		logger.Debug("Killing ffmpeg process with PID: %d", pid)

		killErr := t.cmd.Process.Kill()
		if killErr != nil {
			logger.Error("Error killing ffmpeg process: %v", killErr)
		} else {
			logger.Debug("Successfully killed ffmpeg process with PID: %d", pid)
		}

		t.cmd = nil
	}

	// Clear active streams
	t.activeStreams = make(map[string]time.Time)
	logger.Info("All transcoding processes stopped")
}
