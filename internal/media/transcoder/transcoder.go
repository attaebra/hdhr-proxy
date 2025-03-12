package transcoder

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

	"github.com/attaebra/hdhr-proxy/internal/constants"
	"github.com/attaebra/hdhr-proxy/internal/logger"
	"github.com/attaebra/hdhr-proxy/internal/media/buffer"
	"github.com/attaebra/hdhr-proxy/internal/media/ffmpeg"
	"github.com/attaebra/hdhr-proxy/internal/media/stream"
	"github.com/attaebra/hdhr-proxy/internal/proxy"
	"github.com/attaebra/hdhr-proxy/internal/utils"
)

// Transcoder manages the FFmpeg process for transcoding AC4 to EAC3.
type Transcoder struct {
	FFmpegPath            string
	InputURL              string
	ctx                   context.Context
	cancel                context.CancelFunc
	cmd                   *exec.Cmd
	mutex                 sync.Mutex
	activeStreams         map[string]time.Time // Track active streams by channel ID
	RequestTimeout        time.Duration        // HTTP request timeout
	proxy                 *proxy.HDHRProxy     // Reference to the proxy for API access
	ac4Channels           map[string]bool      // Track which channels have AC4 audio
	connectionActivity    map[string]time.Time
	activityCheckInterval time.Duration
	maxInactivityDuration time.Duration
	activityMutex         sync.Mutex
	stopActivityCheck     context.CancelFunc
	ffmpegProcesses       map[int]string

	// New fields for the improved buffer and streaming modules
	BufferManager *buffer.Manager
	FFmpegConfig  *ffmpeg.Config
	StreamHelper  *stream.Helper
}

// NewTranscoder creates a new transcoder instance.
func NewTranscoder(ffmpegPath string, hdhrIP string) *Transcoder {
	// Ensure the input URL is correctly formatted
	baseURL := fmt.Sprintf("http://%s:%d", hdhrIP, constants.DefaultMediaPort)
	logger.Debug("Using streaming base URL: %s", baseURL)

	// Initialize proxy
	hdhrProxy := proxy.NewHDHRProxy(hdhrIP)

	// Initialize with a configurable request timeout
	var requestTimeout time.Duration
	timeoutStr := os.Getenv("REQUEST_TIMEOUT")
	if timeoutStr != "" {
		// Try to parse the timeout duration
		var err error
		requestTimeout, err = time.ParseDuration(timeoutStr)
		if err == nil {
			logger.Debug("Using custom request timeout: %s", requestTimeout)
		} else {
			logger.Warn("Invalid REQUEST_TIMEOUT format, using no timeout")
			requestTimeout = 0
		}
	} else {
		logger.Debug("No timeout configured, streaming will continue indefinitely")
		requestTimeout = 0
	}

	// Create context for the activity checker
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize the buffer manager with optimized sizes
	bufferManager := buffer.NewManager(
		4*1024*1024, // 4MB ring buffer for smoother playback
		64*1024,     // 64KB read buffer chunks
		128*1024,    // 128KB write buffer chunks
	)

	// Create the optimized FFmpeg config
	ffmpegConfig := ffmpeg.NewOptimizedConfig()

	// Create stream helper
	streamHelper := stream.NewHelper(bufferManager)

	t := &Transcoder{
		FFmpegPath:            ffmpegPath,
		proxy:                 hdhrProxy,
		activeStreams:         make(map[string]time.Time),
		ac4Channels:           make(map[string]bool),
		ffmpegProcesses:       make(map[int]string),
		InputURL:              baseURL,
		RequestTimeout:        requestTimeout,
		connectionActivity:    make(map[string]time.Time),
		activityCheckInterval: 30 * time.Second, // Check every 30 seconds
		maxInactivityDuration: 2 * time.Minute,  // Kill after 2 minutes of inactivity
		ctx:                   ctx,
		stopActivityCheck:     cancel,

		// Initialize new modules
		BufferManager: bufferManager,
		FFmpegConfig:  ffmpegConfig,
		StreamHelper:  streamHelper,
	}

	// Fetch the channel lineup to identify AC4 channels
	err := t.fetchAC4Channels()
	if err != nil {
		logger.Warn("Failed to fetch AC4 channels: %v", err)
	}

	// Start the activity checker goroutine
	go t.checkInactiveConnections()

	return t
}

// fetchAC4Channels fetches the lineup from the HDHomeRun and identifies channels with AC4 audio.
func (t *Transcoder) fetchAC4Channels() error {
	defer utils.TimeOperation("Fetch AC4 channels")()

	// Create an HTTP client
	client := &http.Client{
		Timeout: 5 * time.Second, // Add a reasonable timeout for API requests
	}

	// Create the request
	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/lineup.json", t.proxy.HDHRIP), nil)
	if err != nil {
		return utils.LogAndWrapError(err, "failed to create request")
	}

	logger.Debug("Fetching lineup from %s", t.proxy.HDHRIP)

	// Execute the request
	resp, err := client.Do(req)
	if err != nil {
		return utils.LogAndWrapError(err, "failed to fetch lineup from %s", t.proxy.HDHRIP)
	}
	defer utils.CloseWithLogging(resp.Body, "response body")

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return utils.LogAndWrapError(fmt.Errorf("HTTP status %d", resp.StatusCode), "invalid response from HDHomeRun")
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
		return utils.LogAndWrapError(err, "failed to parse lineup")
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

// getDefaultString returns the default value if the input is empty.
func getDefaultString(input, defaultVal string) string {
	if input == "" {
		return defaultVal
	}
	return input
}

// isAC4Channel checks if a channel uses AC4 audio codec.
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

// DirectStreamChannel streams the channel directly without transcoding.
func (t *Transcoder) DirectStreamChannel(w http.ResponseWriter, r *http.Request, channel string) error {
	start := time.Now()

	// Track this stream in our active streams
	t.mutex.Lock()
	t.activeStreams[channel] = start
	activeCount := len(t.activeStreams)
	t.mutex.Unlock()

	// Update activity timestamp
	t.updateActivityTimestamp(channel)

	logger.Info("Direct streaming (no transcode) for channel: %s (active streams: %d)", channel, activeCount)
	logger.Debug("Using input URL: %s/auto/v%s", t.InputURL, channel)

	// Create a context that will be canceled when the client disconnects
	ctx, cancel := context.WithCancel(r.Context())

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

	// Create the request
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
		statusMsg := fmt.Sprintf("Invalid response from HDHomeRun: %d", resp.StatusCode)
		logger.Error("Invalid response from HDHomeRun: %d", resp.StatusCode)
		http.Error(w, statusMsg, http.StatusBadGateway)
		return fmt.Errorf("invalid response from HDHomeRun: %d", resp.StatusCode)
	}

	// Log content type and headers for debugging
	logger.Debug("Response content type: %s", resp.Header.Get("Content-Type"))
	logger.Debug("Response headers: %v", resp.Header)

	// Set appropriate headers for streaming
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))

	// Use our buffered copy for smoother streaming instead of simple io.Copy
	logger.Debug("Starting buffered stream copy from HDHomeRun to response for channel %s...", channel)
	bytesCopied, err := t.StreamHelper.BufferedCopy(ctx, w, resp.Body)
	if err != nil {
		if strings.Contains(err.Error(), "connection reset by peer") ||
			strings.Contains(err.Error(), "broken pipe") {
			logger.Debug("Client disconnected during direct stream for channel %s: %v", channel, err)
			return nil // Client disconnection is not an error we need to report
		}
		logger.Error("Error in buffered copy from HDHomeRun to response: %v", err)
		return fmt.Errorf("stream interrupted: %w", err)
	}

	// Log buffer stats for debugging
	used, capacity := t.StreamHelper.GetBufferStatus()
	logger.Debug("Finished direct stream copy, bytes copied: %d, final buffer status: %d/%d bytes",
		bytesCopied, used, capacity)

	return nil
}

// TranscodeChannel starts the ffmpeg process to transcode from AC4 to EAC3.
func (t *Transcoder) TranscodeChannel(w http.ResponseWriter, r *http.Request, channel string) error {
	defer utils.TimeOperation(fmt.Sprintf("Transcoding channel %s", channel))()

	start := time.Now()

	// Track this stream in our active streams
	t.mutex.Lock()
	t.activeStreams[channel] = start
	activeCount := len(t.activeStreams)
	t.mutex.Unlock()

	// Update activity timestamp
	t.updateActivityTimestamp(channel)

	logger.Info("Starting transcoding for channel: %s (active streams: %d)", channel, activeCount)
	logger.Debug("Using input URL: %s/auto/v%s", t.InputURL, channel)

	// Create a context that will be canceled when the client disconnects
	ctx, cancel := context.WithCancel(r.Context())

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

	// Create the request
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
		statusMsg := fmt.Sprintf("Invalid response from HDHomeRun: %d", resp.StatusCode)
		logger.Error("Invalid response from HDHomeRun: %d", resp.StatusCode)
		http.Error(w, statusMsg, http.StatusBadGateway)
		return fmt.Errorf("invalid response from HDHomeRun: %d", resp.StatusCode)
	}

	// Log content type and headers for debugging
	logger.Debug("Response content type: %s", resp.Header.Get("Content-Type"))
	logger.Debug("Response headers: %v", resp.Header)

	// Start FFmpeg to transcode the stream
	return t.startFFmpeg(ctx, w, resp.Body, channel)
}

// Stop stops the transcoding process.
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

// CreateMediaHandler returns a http.Handler for the media endpoints.
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
	mux.HandleFunc("/status", func(w http.ResponseWriter, _ *http.Request) {
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

// StopAllTranscoding stops any running transcoding processes.
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

// checkInactiveConnections periodically checks for and cleans up inactive connections.
func (t *Transcoder) checkInactiveConnections() {
	ticker := time.NewTicker(t.activityCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			t.cleanupInactiveStreams()
		}
	}
}

// cleanupInactiveStreams identifies and removes streams that have been inactive for too long.
func (t *Transcoder) cleanupInactiveStreams() {
	now := time.Now()

	t.activityMutex.Lock()
	activityCopy := make(map[string]time.Time)
	for k, v := range t.connectionActivity {
		activityCopy[k] = v
	}
	t.activityMutex.Unlock()

	t.mutex.Lock()
	streamCopy := make(map[string]time.Time)
	for k, v := range t.activeStreams {
		streamCopy[k] = v
	}
	t.mutex.Unlock()

	// Check for inactive streams
	for channel, lastActivity := range activityCopy {
		if _, isActive := streamCopy[channel]; isActive {
			inactiveDuration := now.Sub(lastActivity)
			if inactiveDuration > t.maxInactivityDuration {
				logger.Warn("Detected stale connection for channel %s (inactive for %.1f seconds) - forcing cleanup",
					channel, inactiveDuration.Seconds())

				// Force cleanup
				t.StopActiveStream(channel)

				t.activityMutex.Lock()
				delete(t.connectionActivity, channel)
				t.activityMutex.Unlock()

				logger.Info("Forced cleanup of inactive stream for channel %s", channel)
			}
		} else {
			// Clean up activity tracking for channels that are no longer active
			t.activityMutex.Lock()
			delete(t.connectionActivity, channel)
			t.activityMutex.Unlock()
		}
	}
}

// updateActivityTimestamp records the last activity time for a channel.
func (t *Transcoder) updateActivityTimestamp(channel string) {
	t.activityMutex.Lock()
	t.connectionActivity[channel] = time.Now()
	t.activityMutex.Unlock()
}

// startFFmpeg starts an FFmpeg process for transcoding with context as first parameter.
func (t *Transcoder) startFFmpeg(ctx context.Context, w http.ResponseWriter, r io.Reader, channel string) error {
	logger.Debug("Setting up ffmpeg command with path: %s", t.FFmpegPath)

	// Validate the FFmpeg path to prevent command injection
	if err := utils.ValidateExecutable(t.FFmpegPath); err != nil {
		logger.Error("Invalid FFmpeg executable: %v", err)
		http.Error(w, "FFmpeg configuration error", http.StatusInternalServerError)
		return fmt.Errorf("invalid FFmpeg executable: %w", err)
	}

	// Use the optimized FFmpeg config with improved parameters
	cmd := exec.CommandContext(ctx, t.FFmpegPath, t.FFmpegConfig.BuildArgs()...)

	// Get pipes for stdin, stdout, and stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		logger.Error("Failed to get stdin pipe: %v", err)
		http.Error(w, "Failed to start ffmpeg", http.StatusInternalServerError)
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logger.Error("Failed to get stdout pipe: %v", err)
		http.Error(w, "Failed to start ffmpeg", http.StatusInternalServerError)
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		logger.Error("Failed to get stderr pipe: %v", err)
		http.Error(w, "Failed to start ffmpeg", http.StatusInternalServerError)
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	// Start FFmpeg
	logger.Debug("Starting ffmpeg process...")
	ffmpegStart := time.Now()
	if err := cmd.Start(); err != nil {
		logger.Error("Failed to start ffmpeg: %v", err)
		http.Error(w, "Failed to start ffmpeg", http.StatusInternalServerError)
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	ffmpegPid := cmd.Process.Pid
	logger.Debug("ffmpeg process started successfully with PID: %d in %d ms",
		ffmpegPid, time.Since(ffmpegStart).Milliseconds())

	// Store the ffmpeg process ID
	t.mutex.Lock()
	t.ffmpegProcesses[ffmpegPid] = channel
	t.mutex.Unlock()

	// Set up a defer to kill the ffmpeg process if needed
	defer func() {
		logger.Debug("Killing ffmpeg process with PID: %d", ffmpegPid)
		if err := cmd.Process.Kill(); err != nil {
			logger.Error("Failed to kill ffmpeg process: %v", err)
		}

		t.mutex.Lock()
		delete(t.ffmpegProcesses, ffmpegPid)
		t.mutex.Unlock()
	}()

	// Create a scanner to read from stderr for debugging
	scanner := bufio.NewScanner(stderr)
	go func() {
		for scanner.Scan() {
			logger.Debug("ffmpeg[%d]: %s", ffmpegPid, scanner.Text())
		}
	}()

	// Set appropriate content type header
	w.Header().Set("Content-Type", "video/MP2T")

	// Set up a goroutine to copy from HDHomeRun to ffmpeg
	go func() {
		defer stdin.Close()
		logger.Debug("Starting stream copy from HDHomeRun to ffmpeg for channel %s...", channel)
		// Get a buffer from the pool for reading
		readBuf := t.BufferManager.GetReadBuffer()
		defer t.BufferManager.ReleaseBuffer(readBuf)

		// Use a buffered copy approach
		var totalCopied int64
		for {
			select {
			case <-ctx.Done():
				logger.Debug("Context canceled during HDHomeRun to ffmpeg copy")
				return
			default:
				// Read from the source
				n, err := r.Read(readBuf.B)
				if n > 0 {
					// Write to ffmpeg stdin
					_, werr := stdin.Write(readBuf.B[:n])
					totalCopied += int64(n)
					if werr != nil {
						if strings.Contains(werr.Error(), "broken pipe") {
							logger.Debug("FFmpeg pipe closed during write")
						} else {
							logger.Error("Error writing to ffmpeg: %v", werr)
						}
						return
					}
				}
				if err != nil {
					if err != io.EOF &&
						!strings.Contains(err.Error(), "connection reset by peer") &&
						!strings.Contains(err.Error(), "broken pipe") {
						logger.Error("Error reading from HDHomeRun: %v", err)
					}
					return
				}
			}
		}
	}()

	// Use the stream helper for buffered copying from ffmpeg to the client
	logger.Debug("Starting buffered stream copy from ffmpeg to response for channel %s...", channel)
	bytesCopied, err := t.StreamHelper.BufferedCopy(ctx, w, stdout)
	if err != nil {
		if strings.Contains(err.Error(), "connection reset by peer") ||
			strings.Contains(err.Error(), "broken pipe") {
			logger.Debug("Client disconnected during ffmpeg to client copy for channel %s: %v", channel, err)
			return nil // Client disconnection is not an error we need to report
		}
		logger.Error("Error in buffered copy from ffmpeg to response: %v", err)
		return fmt.Errorf("failed to copy from ffmpeg to response: %w", err)
	}

	// Log buffer stats for debugging
	used, capacity := t.StreamHelper.GetBufferStatus()
	logger.Debug("Finished buffered copy from ffmpeg to response, bytes copied: %d, final buffer status: %d/%d bytes",
		bytesCopied, used, capacity)

	// Wait for ffmpeg to exit
	if err := cmd.Wait(); err != nil {
		logger.Error("ffmpeg process exited with error: %v", err)
		return err
	}

	logger.Debug("Transcoding completed for channel %s", channel)
	return nil
}

// StopActiveStream stops and cleans up resources for a specific channel stream.
func (t *Transcoder) StopActiveStream(channel string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// Remove the active stream
	delete(t.activeStreams, channel)

	// Look for any ffmpeg processes for this channel and stop them
	for pid, ch := range t.ffmpegProcesses {
		if ch == channel {
			logger.Debug("Killing ffmpeg process with PID: %d for channel %s", pid, channel)
			process, err := os.FindProcess(pid)
			if err == nil {
				if killErr := process.Kill(); killErr != nil {
					logger.Error("Error killing ffmpeg process: %v", killErr)
				} else {
					logger.Debug("Successfully killed ffmpeg process with PID: %d", pid)
				}
			}
			delete(t.ffmpegProcesses, pid)
		}
	}

	logger.Info("Stopped active stream for channel %s", channel)
}

// Shutdown performs a graceful shutdown of the transcoder and all its resources.
func (t *Transcoder) Shutdown() {
	defer utils.TimeOperation("Shutdown transcoder")()
	logger.Info("Stopping transcoder gracefully")

	// Stop the activity checker
	if t.stopActivityCheck != nil {
		t.stopActivityCheck()
	}

	// Stop all processes
	t.StopAllTranscoding()
}
