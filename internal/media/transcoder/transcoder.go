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
	"sync/atomic"
	"time"

	"github.com/attaebra/hdhr-proxy/internal/config"
	"github.com/attaebra/hdhr-proxy/internal/interfaces"
	"github.com/attaebra/hdhr-proxy/internal/logger"

	"github.com/attaebra/hdhr-proxy/internal/utils"
)

// Dependencies holds all dependencies needed for transcoder initialization.
type Dependencies struct {
	Config            *config.Config
	Logger            interfaces.Logger
	HTTPClient        interfaces.Client
	StreamClient      interfaces.Client
	FFmpegConfig      interfaces.Config
	StreamHelper      interfaces.Streamer
	HDHRProxy         interfaces.Proxy
	SecurityValidator interfaces.SecurityValidator
}

// Impl manages the FFmpeg process for transcoding AC4 to EAC3.
type Impl struct {
	FFmpegPath            string
	InputURL              string
	ctx                   context.Context
	cancel                context.CancelFunc
	cmd                   *exec.Cmd
	mutex                 sync.Mutex
	activeStreams         map[string]time.Time // Track active streams by channel ID
	proxy                 interfaces.Proxy     // Reference to the proxy for API access
	ac4Channels           map[string]bool      // Track which channels have AC4 audio
	connectionActivity    map[string]time.Time
	activityCheckInterval time.Duration
	maxInactivityDuration time.Duration
	activityMutex         sync.Mutex
	stopActivityCheck     context.CancelFunc
	ffmpegProcesses       map[string]int // Map channel to PID (changed from int->string to string->int)
	monitoringActive      bool           // Flag to track if monitoring is active

	// Injected dependencies
	logger            interfaces.Logger            // Structured logger via DI
	FFmpegConfig      interfaces.Config            // FFmpeg configuration
	StreamHelper      interfaces.Streamer          // Stream processing helper
	apiClient         interfaces.Client            // For API requests with timeouts
	streamClient      interfaces.Client            // for streaming with no timeout
	securityValidator interfaces.SecurityValidator // Security validation
}

// Ensure Impl implements the Transcoder interface.
var _ interfaces.Transcoder = (*Impl)(nil)

// Transcoder creates a new transcoder instance with injected dependencies.
func Transcoder(deps *Dependencies) (interfaces.Transcoder, error) {
	// Validate FFmpeg path
	if err := deps.SecurityValidator.ValidateExecutable(deps.Config.FFmpegPath); err != nil {
		return nil, fmt.Errorf("invalid FFmpeg path: %w", err)
	}

	// Create context for the activity checker
	ctx, cancel := context.WithCancel(context.Background())

	// Ensure the input URL is correctly formatted
	baseURL := fmt.Sprintf("http://%s:%d", deps.Config.HDHomeRunIP, deps.Config.MediaPort)
	// Note: we'll use the injected logger after t is created

	t := &Impl{
		FFmpegPath:            deps.Config.FFmpegPath,
		proxy:                 deps.HDHRProxy,
		activeStreams:         make(map[string]time.Time),
		ac4Channels:           make(map[string]bool),
		ffmpegProcesses:       make(map[string]int),
		InputURL:              baseURL,
		connectionActivity:    make(map[string]time.Time),
		activityCheckInterval: deps.Config.ActivityCheckInterval,
		maxInactivityDuration: deps.Config.MaxInactivityDuration,
		ctx:                   ctx,
		cancel:                cancel,
		monitoringActive:      false,

		// Initialize injected dependencies
		logger:            deps.Logger,
		FFmpegConfig:      deps.FFmpegConfig,
		StreamHelper:      deps.StreamHelper,
		apiClient:         deps.HTTPClient,
		streamClient:      deps.StreamClient,
		securityValidator: deps.SecurityValidator,
	}

	// Fetch the channel lineup to identify AC4 channels
	err := t.fetchAC4Channels()
	if err != nil {
		t.logger.Warn("⚠️  Failed to fetch AC4 channels", logger.ErrorField("error", err))
	}

	// Log the base URL after logger is available
	t.logger.Debug("🌐 Using streaming base URL", logger.String("base_url", baseURL))

	// Start the connection monitor
	t.startConnectionMonitor()

	return t, nil
}

// fetchAC4Channels fetches the lineup from the HDHomeRun and identifies channels with AC4 audio.
func (t *Impl) fetchAC4Channels() error {
	defer utils.TimeOperation("Fetch AC4 channels")()

	// Create the request
	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/lineup.json", t.proxy.GetHDHRIP()), nil)
	if err != nil {
		return utils.LogAndWrapError(err, "failed to create request")
	}

	t.logger.Debug("📡 Fetching channel lineup", logger.String("hdhr_ip", t.proxy.GetHDHRIP()))

	// Execute the request using the optimized API client
	resp, err := t.apiClient.Do(req)
	if err != nil {
		return utils.LogAndWrapError(err, "failed to fetch lineup from %s", t.proxy.GetHDHRIP())
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
			t.logger.Info("🎵 Identified AC4 audio channel",
				logger.String("channel", channel.GuideNumber),
				logger.String("name", channel.GuideName),
				logger.String("audio_codec", channel.AudioCodec),
				logger.String("video_codec", channel.VideoCodec))
		} else {
			t.logger.Debug("📺 Regular channel",
				logger.String("channel", channel.GuideNumber),
				logger.String("name", channel.GuideName),
				logger.String("audio_codec", getDefaultString(channel.AudioCodec, "Unknown")),
				logger.String("video_codec", getDefaultString(channel.VideoCodec, "Unknown")))
		}
	}

	t.logger.Info("📊 Channel lineup analyzed",
		logger.Int("ac4_channels", ac4Count),
		logger.Int("total_channels", len(lineup)))

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
func (t *Impl) isAC4Channel(channel string) bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	isAC4, exists := t.ac4Channels[channel]
	if !exists {
		// If we don't know, assume it might have AC4 to be safe
		t.logger.Debug("❓ Unknown channel, assuming AC4",
			logger.String("channel", channel))
		return true
	}
	return isAC4
}

// StreamSetup contains the result of setting up a stream connection.
type StreamSetup struct {
	Response     *http.Response
	Context      context.Context
	Cancel       context.CancelFunc
	ClientCancel context.CancelFunc
	StartTime    time.Time
}

// setupStreamConnection handles common stream setup logic for both direct and transcoded streams.
func (t *Impl) setupStreamConnection(w http.ResponseWriter, r *http.Request, channel string, streamType string) (*StreamSetup, error) {
	start := time.Now()

	// Track this stream in our active streams
	t.mutex.Lock()
	t.activeStreams[channel] = start
	activeCount := len(t.activeStreams)
	t.mutex.Unlock()

	// Update activity timestamp
	t.updateActivityTimestamp(channel)

	t.logger.Info("▶️  Stream setup",
		logger.String("type", streamType),
		logger.String("channel", channel),
		logger.Int("active_streams", activeCount))
	t.logger.Debug("🔗 Stream connection",
		logger.String("input_url", fmt.Sprintf("%s/auto/v%s", t.InputURL, channel)))

	// Create a context that will be canceled when the client disconnects
	ctx, cancel := context.WithCancel(r.Context())

	// Use the streaming client (no timeout) for media streaming operations
	client := t.streamClient
	t.logger.Debug("🚰 Using streaming client with no timeout")

	// Create the request
	sourceURL := fmt.Sprintf("%s/auto/v%s", t.InputURL, channel)
	t.logger.Debug("🌐 Connecting to source", logger.String("url", sourceURL))
	req, err := http.NewRequestWithContext(ctx, "GET", sourceURL, nil)
	if err != nil {
		cancel()
		t.logger.Error("❌ Failed to create HTTP request", logger.ErrorField("error", err))
		http.Error(w, "Failed to create HTTP request", http.StatusInternalServerError)
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add default headers
	req.Header.Set("User-Agent", "hdhr-proxy/1.0")

	// Execute the request
	t.logger.Debug("📡 Sending request to HDHomeRun...")
	connStart := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		cancel()
		t.logger.Error("❌ Failed to fetch stream", logger.ErrorField("error", err))
		http.Error(w, "Failed to fetch stream from HDHomeRun", http.StatusBadGateway)
		return nil, fmt.Errorf("failed to fetch stream: %w", err)
	}
	t.logger.Debug("✅ Connected to HDHomeRun", logger.Duration("connect_time", time.Since(connStart)))

	// Check response status
	t.logger.Debug("📨 Received response", logger.Int("status_code", resp.StatusCode))
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		cancel()
		statusMsg := fmt.Sprintf("Invalid response from HDHomeRun: %d", resp.StatusCode)
		t.logger.Error("❌ Invalid response from HDHomeRun", logger.Int("status_code", resp.StatusCode))
		http.Error(w, statusMsg, http.StatusBadGateway)
		return nil, fmt.Errorf("invalid response from HDHomeRun: %d", resp.StatusCode)
	}

	// Log response details
	t.logger.Debug("📄 Response details",
		logger.String("content_type", resp.Header.Get("Content-Type")),
		logger.Any("headers", resp.Header))

	// Set appropriate headers for streaming
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))

	// Create client context for disconnect detection
	clientCtx, clientCancel := context.WithCancel(ctx)

	// Set up goroutine to detect client disconnection
	go func() {
		<-clientCtx.Done()
		t.logger.Debug("🔌 Client disconnected, cleaning up resources",
			logger.String("channel", channel))
		t.StopActiveStream(channel)
	}()

	return &StreamSetup{
		Response:     resp,
		Context:      clientCtx,
		Cancel:       cancel,
		ClientCancel: clientCancel,
		StartTime:    start,
	}, nil
}

// cleanupStream handles cleanup after streaming is complete.
func (t *Impl) cleanupStream(setup *StreamSetup, channel string, streamType string) {
	if r := recover(); r != nil {
		t.logger.Error("🚨 Recovered from panic",
			logger.String("stream_type", streamType),
			logger.Any("panic", r),
			logger.String("stack", string(debug.Stack())))
	}

	// Cancel contexts to release resources
	setup.Cancel()
	setup.ClientCancel()

	if setup.Response != nil {
		setup.Response.Body.Close()
	}

	// Remove this stream from active streams
	t.mutex.Lock()
	delete(t.activeStreams, channel)
	duration := time.Since(setup.StartTime).Seconds()
	t.mutex.Unlock()

	t.logger.Info("⏹️  Stream session ended",
		logger.String("type", streamType),
		logger.String("channel", channel),
		logger.Duration("duration", time.Duration(duration*float64(time.Second))))
}

// DirectStreamChannel streams the channel directly without transcoding.
func (t *Impl) DirectStreamChannel(w http.ResponseWriter, r *http.Request, channel string) error {
	// Setup stream connection using shared helper
	setup, err := t.setupStreamConnection(w, r, channel, "Direct streaming (no transcode)")
	if err != nil {
		return err
	}

	// Cleanup when done
	defer t.cleanupStream(setup, channel, "Direct streaming")

	// Use our stream copy instead of simple io.Copy
	t.logger.Debug("📺 Starting direct stream copy", logger.String("channel", channel))
	bytesCopied, err := t.StreamHelper.CopyWithActivityUpdate(setup.Context, w, setup.Response.Body, func() {
		// Update activity timestamp whenever data is sent to the client
		t.updateActivityTimestamp(channel)
	})

	if err != nil {
		if strings.Contains(err.Error(), "connection reset by peer") ||
			strings.Contains(err.Error(), "broken pipe") {
			t.logger.Debug("🔌 Client disconnected during direct stream",
				logger.String("channel", channel),
				logger.ErrorField("error", err))
			// Ensure we clean up resources when the client disconnects
			t.StopActiveStream(channel)
			return nil // Client disconnection is not an error we need to report
		}
		t.logger.Error("❌ Stream copy error", logger.ErrorField("error", err))
		return fmt.Errorf("stream interrupted: %w", err)
	}

	t.logger.Debug("✅ Direct stream completed",
		logger.String("channel", channel),
		logger.Int64("bytes_copied", bytesCopied))
	return nil
}

// TranscodeChannel starts the ffmpeg process to transcode from AC4 to EAC3.
func (t *Impl) TranscodeChannel(w http.ResponseWriter, r *http.Request, channel string) error {
	defer utils.TimeOperation(fmt.Sprintf("Transcoding channel %s", channel))()

	// Setup stream connection using shared helper
	setup, err := t.setupStreamConnection(w, r, channel, "Starting transcoding")
	if err != nil {
		return err
	}

	// Cleanup when done
	defer t.cleanupStream(setup, channel, "Transcoding")

	// Start FFmpeg to transcode the stream
	return t.startFFmpeg(setup.Context, w, setup.Response.Body, channel)
}

// Stop stops the transcoding process.
func (t *Impl) Stop() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.logger.Info("🛑 Stopping transcoder gracefully")

	if t.cancel != nil {
		t.cancel()
	}

	// Log active streams that are being stopped
	for channel, startTime := range t.activeStreams {
		duration := time.Since(startTime)
		t.logger.Info("⏹️  Stopping active stream",
			logger.String("channel", channel),
			logger.Duration("duration", duration))
	}

	// Clear active streams
	t.activeStreams = make(map[string]time.Time)
}

// MediaHandler returns a http.Handler for the media endpoints.
func (t *Impl) MediaHandler() http.Handler {
	mux := http.NewServeMux()

	// Handle auto/v{channel} requests for channel transcoding
	mux.HandleFunc("/auto/", func(w http.ResponseWriter, r *http.Request) {
		remoteAddr := r.RemoteAddr
		userAgent := r.UserAgent()

		t.logger.Info("📺 Media request received",
			logger.String("method", r.Method),
			logger.String("path", r.URL.Path),
			logger.String("client_ip", remoteAddr),
			logger.String("user_agent", userAgent))

		// Extract channel from URL path
		if !strings.HasPrefix(r.URL.Path, "/auto/v") {
			t.logger.Debug("❌ Invalid path pattern", logger.String("path", r.URL.Path))
			http.NotFound(w, r)
			return
		}

		channel := strings.TrimPrefix(r.URL.Path, "/auto/v")
		if channel == "" {
			t.logger.Warn("⚠️  Empty channel requested", logger.String("client_ip", remoteAddr))
			http.Error(w, "Missing channel number", http.StatusBadRequest)
			return
		}

		// Check if this channel has AC4 audio needing transcoding
		if t.isAC4Channel(channel) {
			t.logger.Info("🎵 AC4 transcoding started",
				logger.String("channel", channel),
				logger.String("from", "AC4"),
				logger.String("to", "EAC3"))
			if err := t.TranscodeChannel(w, r, channel); err != nil {
				t.logger.Error("❌ Transcoding error",
					logger.String("channel", channel),
					logger.ErrorField("error", err))
				// Error already sent to client by TranscodeChannel
			}
		} else {
			// For channels without AC4 audio, stream directly without transcoding
			t.logger.Info("📡 Direct streaming",
				logger.String("channel", channel),
				logger.String("mode", "pass-through"),
				logger.String("reason", "non-AC4 audio"))
			if err := t.DirectStreamChannel(w, r, channel); err != nil {
				t.logger.Error("❌ Direct streaming error",
					logger.String("channel", channel),
					logger.ErrorField("error", err))
				// Error already handled by DirectStreamChannel
			}
		}

		t.logger.Debug("✅ Media handler completed",
			logger.String("channel", channel),
			logger.String("client_ip", remoteAddr))
	})

	// Add a helper function to write output and log it at debug level
	writeOutput := func(w http.ResponseWriter, format string, args ...interface{}) {
		msg := fmt.Sprintf(format, args...)
		t.logger.Debug("📊 Status output", logger.String("content", strings.TrimSpace(msg)))
		fmt.Fprint(w, msg)
	}

	// Status endpoint handler
	mux.HandleFunc("/status", func(w http.ResponseWriter, _ *http.Request) {
		t.logger.Info("📊 Status endpoint accessed")

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
		writeOutput(w, "HDHomeRun Device: %s\n", t.proxy.GetHDHRIP())
		writeOutput(w, "FFmpeg Path: %s\n", t.FFmpegPath)
		writeOutput(w, "Stream Timeout: None (streams indefinitely)\n")
	})

	return mux
}

// StopAllTranscoding stops any running transcoding processes.
func (t *Impl) StopAllTranscoding() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	activeStreams := len(t.activeStreams)
	t.logger.Info("🛑 Stopping all transcoding processes",
		logger.Int("active_streams", activeStreams))

	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}

	if t.cmd != nil && t.cmd.Process != nil {
		pid := t.cmd.Process.Pid
		t.logger.Debug("🔫 Killing ffmpeg process", logger.Int("pid", pid))

		killErr := t.cmd.Process.Kill()
		if killErr != nil {
			t.logger.Error("❌ Error killing ffmpeg process", logger.ErrorField("error", killErr))
		} else {
			t.logger.Debug("✅ Successfully killed ffmpeg process", logger.Int("pid", pid))
		}

		t.cmd = nil
	}

	// Clear active streams
	t.activeStreams = make(map[string]time.Time)
	t.logger.Info("✅ All transcoding processes stopped")
}

// updateActivityTimestamp records the last activity time for a channel.
func (t *Impl) updateActivityTimestamp(channel string) {
	t.activityMutex.Lock()
	t.connectionActivity[channel] = time.Now()
	t.activityMutex.Unlock()
}

// startConnectionMonitor starts a goroutine that periodically checks for inactive connections.
func (t *Impl) startConnectionMonitor() {
	t.mutex.Lock()
	if t.monitoringActive {
		t.mutex.Unlock()
		return
	}
	t.monitoringActive = true
	t.mutex.Unlock()

	t.logger.Info("🔍 Starting connection monitor",
		logger.Duration("check_interval", t.activityCheckInterval),
		logger.Duration("max_inactivity", t.maxInactivityDuration))

	go func() {
		ticker := time.NewTicker(t.activityCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				t.cleanupInactiveStreams()
			case <-t.ctx.Done():
				t.logger.Debug("🔍 Connection monitor stopped")
				return
			}
		}
	}()
}

// cleanupInactiveStreams checks for and cleans up inactive streams.
func (t *Impl) cleanupInactiveStreams() {
	t.activityMutex.Lock()
	now := time.Now()
	inactiveChannels := []string{}

	// First, identify inactive channels
	for channel, lastActivity := range t.connectionActivity {
		inactiveDuration := now.Sub(lastActivity)
		if inactiveDuration > t.maxInactivityDuration {
			t.logger.Info("🕐 Detected inactive stream",
				logger.String("channel", channel),
				logger.Duration("inactive_duration", inactiveDuration))
			inactiveChannels = append(inactiveChannels, channel)
		}
	}
	t.activityMutex.Unlock()

	// Then clean them up
	for _, channel := range inactiveChannels {
		t.logger.Info("🧹 Cleaning up inactive stream", logger.String("channel", channel))
		t.StopActiveStream(channel)

		// Also remove from activity tracking
		t.activityMutex.Lock()
		delete(t.connectionActivity, channel)
		t.activityMutex.Unlock()
	}
}

// startFFmpeg starts an FFmpeg process for transcoding with context as first parameter.
func (t *Impl) startFFmpeg(ctx context.Context, w http.ResponseWriter, r io.Reader, channel string) error {
	t.logger.Debug("🎬 Setting up ffmpeg command", logger.String("ffmpeg_path", t.FFmpegPath))

	// Validate the FFmpeg path to prevent command injection
	if err := t.securityValidator.ValidateExecutable(t.FFmpegPath); err != nil {
		t.logger.Error("❌ Invalid FFmpeg executable", logger.ErrorField("error", err))
		http.Error(w, "FFmpeg configuration error", http.StatusInternalServerError)
		return fmt.Errorf("invalid FFmpeg executable: %w", err)
	}

	// Use the optimized FFmpeg config with improved parameters
	cmd := exec.CommandContext(ctx, t.FFmpegPath, t.FFmpegConfig.BuildArgs()...)

	// Get pipes for stdin, stdout, and stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.logger.Error("❌ Failed to get stdin pipe", logger.ErrorField("error", err))
		http.Error(w, "Failed to start ffmpeg", http.StatusInternalServerError)
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.logger.Error("❌ Failed to get stdout pipe", logger.ErrorField("error", err))
		http.Error(w, "Failed to start ffmpeg", http.StatusInternalServerError)
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.logger.Error("❌ Failed to get stderr pipe", logger.ErrorField("error", err))
		http.Error(w, "Failed to start ffmpeg", http.StatusInternalServerError)
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	// Start FFmpeg
	t.logger.Debug("🚀 Starting ffmpeg process...")
	ffmpegStart := time.Now()
	if err := cmd.Start(); err != nil {
		t.logger.Error("❌ Failed to start ffmpeg", logger.ErrorField("error", err))
		http.Error(w, "Failed to start ffmpeg", http.StatusInternalServerError)
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	ffmpegPid := cmd.Process.Pid
	t.logger.Debug("✅ ffmpeg process started",
		logger.Int("pid", ffmpegPid),
		logger.Duration("startup_time", time.Since(ffmpegStart)))

	// Store the ffmpeg process ID
	t.mutex.Lock()
	t.ffmpegProcesses[channel] = ffmpegPid // Changed to store PID by channel
	t.mutex.Unlock()

	// Set up a defer to kill the ffmpeg process if needed
	var cleanupDone int32 // Atomic flag to prevent double cleanup
	defer func() {
		// Use atomic CAS to ensure cleanup only happens once
		if atomic.CompareAndSwapInt32(&cleanupDone, 0, 1) {
			t.logger.Debug("🔫 Cleaning up ffmpeg process", logger.Int("pid", ffmpegPid))

			// Check if process is still alive before attempting to kill
			if process := cmd.Process; process != nil {
				// Try to get process state first
				if processState := cmd.ProcessState; processState == nil || !processState.Exited() {
					if err := process.Kill(); err != nil {
						// Only log error if it's not "process already finished"
						if !strings.Contains(err.Error(), "process already finished") &&
							!strings.Contains(err.Error(), "no such process") {
							t.logger.Error("❌ Failed to kill ffmpeg process", logger.ErrorField("error", err))
						}
					} else {
						t.logger.Debug("✅ Successfully killed ffmpeg process", logger.Int("pid", ffmpegPid))
					}
				}
			}

			t.mutex.Lock()
			delete(t.ffmpegProcesses, channel)
			t.mutex.Unlock()
		}
	}()

	// Create a scanner to read from stderr for debugging
	scanner := bufio.NewScanner(stderr)
	var ac4ErrorCount int32                     // Total AC4 errors for logging
	var consecutiveErrors int32                 // Consecutive errors in a short timeframe
	var lastErrorTime int64                     // Timestamp of last error (Unix nanoseconds)
	const errorResetInterval = 30 * time.Second // Reset consecutive counter after 30 seconds
	const maxConsecutiveErrors = 20             // Allow up to 20 consecutive errors before warning

	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			t.logger.Debug("🎬 ffmpeg output",
				logger.Int("pid", ffmpegPid),
				logger.String("output", line))

			// Detect AC4 decoding errors specifically
			if strings.Contains(line, "[ac4 @") &&
				(strings.Contains(line, "substream audio data overread") ||
					strings.Contains(line, "Invalid data found when processing input")) {

				now := time.Now().UnixNano()
				lastError := atomic.LoadInt64(&lastErrorTime)

				// Reset consecutive counter if enough time has passed since last error
				if now-lastError > int64(errorResetInterval) {
					atomic.StoreInt32(&consecutiveErrors, 0)
				}

				totalCount := atomic.AddInt32(&ac4ErrorCount, 1)
				consecutiveCount := atomic.AddInt32(&consecutiveErrors, 1)
				atomic.StoreInt64(&lastErrorTime, now)

				// Extract just the error type for cleaner logging
				var errorType string
				switch {
				case strings.Contains(line, "substream audio data overread"):
					// Extract the number if present: "substream audio data overread: 5"
					if idx := strings.Index(line, "substream audio data overread"); idx != -1 {
						remaining := line[idx:]
						if colonIdx := strings.Index(remaining, ":"); colonIdx != -1 {
							errorType = strings.TrimSpace(remaining[:colonIdx+2]) // Include the colon and number
						} else {
							errorType = "substream audio data overread"
						}
					}
				case strings.Contains(line, "Invalid data found when processing input"):
					errorType = "invalid data in input stream"
				default:
					errorType = "unknown AC4 error"
				}

				// Log with different severity based on consecutive errors (with emojis!)
				switch {
				case consecutiveCount <= 5:
					t.logger.Debug("🔧 AC4 decoding error",
						logger.String("channel", channel),
						logger.String("error_type", errorType),
						logger.Int("total_errors", int(totalCount)),
						logger.Int("consecutive", int(consecutiveCount)))
				case consecutiveCount <= maxConsecutiveErrors:
					t.logger.Warn("⚠️  AC4 error rate increasing",
						logger.String("channel", channel),
						logger.String("error_type", errorType),
						logger.Int("total_errors", int(totalCount)),
						logger.Int("consecutive", int(consecutiveCount)))
				default:
					t.logger.Error("🚨 High AC4 error rate - stream quality issues",
						logger.String("channel", channel),
						logger.String("error_type", errorType),
						logger.Int("total_errors", int(totalCount)),
						logger.Int("consecutive", int(consecutiveCount)),
						logger.String("recommendation", "Check signal quality"))
				}
			}

			// Log other critical FFmpeg errors (with better sampling)
			if strings.Contains(line, "Error") && !strings.Contains(line, "[ac4 @") {
				t.logger.Error("💥 FFmpeg critical error",
					logger.String("channel", channel),
					logger.Int("pid", ffmpegPid),
					logger.String("error_message", line))
			}
		}
	}()

	// Set appropriate content type header
	w.Header().Set("Content-Type", "video/MP2T")

	// Set up a goroutine to copy from HDHomeRun to ffmpeg
	go func() {
		defer stdin.Close()
		t.logger.Debug("📺 Starting HDHomeRun → FFmpeg copy", logger.String("channel", channel))
		// Use a simple buffer for reading
		readBuf := make([]byte, 64*1024) // 64KB buffer

		// Use a buffered copy approach
		var totalCopied int64
		for {
			select {
			case <-ctx.Done():
				t.logger.Debug("🔄 Context canceled during HDHomeRun → FFmpeg copy")
				return
			default:
				// Read from the source
				n, err := r.Read(readBuf)
				if n > 0 {
					// Write to ffmpeg stdin
					_, werr := stdin.Write(readBuf[:n])
					totalCopied += int64(n)
					if werr != nil {
						if strings.Contains(werr.Error(), "broken pipe") {
							t.logger.Debug("🚰 FFmpeg pipe closed during write")
						} else {
							t.logger.Error("❌ Error writing to ffmpeg", logger.ErrorField("error", werr))
						}
						return
					}
				}
				if err != nil {
					if err != io.EOF &&
						!strings.Contains(err.Error(), "connection reset by peer") &&
						!strings.Contains(err.Error(), "broken pipe") {
						t.logger.Error("❌ Error reading from HDHomeRun", logger.ErrorField("error", err))
					}
					return
				}
			}
		}
	}()

	// Create a context that will be canceled when the client disconnects
	clientCtx, clientCancel := context.WithCancel(ctx)

	// Set up a goroutine to detect client disconnection
	go func() {
		<-clientCtx.Done()
		t.logger.Debug("🔌 Client disconnected, cleaning up FFmpeg resources",
			logger.String("channel", channel))
		t.StopActiveStream(channel)
	}()

	// Make sure we cancel the client context when we're done
	defer clientCancel()

	// Use the stream helper for copying from ffmpeg to the client
	t.logger.Debug("🎬 Starting FFmpeg → Client copy", logger.String("channel", channel))
	bytesCopied, err := t.StreamHelper.CopyWithActivityUpdate(clientCtx, w, stdout, func() {
		// Update activity timestamp whenever data is sent to the client
		t.updateActivityTimestamp(channel)
	})

	if err != nil {
		if strings.Contains(err.Error(), "connection reset by peer") ||
			strings.Contains(err.Error(), "broken pipe") {
			t.logger.Debug("🔌 Client disconnected during FFmpeg → Client copy",
				logger.String("channel", channel),
				logger.ErrorField("error", err))
			// Ensure we clean up resources when the client disconnects
			t.StopActiveStream(channel)
			return nil // Client disconnection is not an error we need to report
		}
		t.logger.Error("❌ FFmpeg → Client copy error", logger.ErrorField("error", err))
		return fmt.Errorf("failed to copy from ffmpeg to response: %w", err)
	}

	t.logger.Debug("✅ FFmpeg → Client copy completed",
		logger.String("channel", channel),
		logger.Int64("bytes_copied", bytesCopied))

	// Wait for ffmpeg to exit
	if err := cmd.Wait(); err != nil {
		// For AC4 streams, decoding errors are common and expected in live TV
		// We should never terminate the stream just because of AC4 decoding errors
		finalErrorCount := atomic.LoadInt32(&ac4ErrorCount)
		if finalErrorCount > 0 {
			t.logger.Info("ℹ️  FFmpeg process ended with AC4 decoding errors (normal for live AC4)",
				logger.String("channel", channel),
				logger.Int("ac4_errors", int(finalErrorCount)))
			// AC4 errors are not considered failures for continuous streaming
			return nil
		}

		// Only treat non-AC4 errors as actual failures
		t.logger.Error("❌ FFmpeg process failed with non-AC4 error",
			logger.String("channel", channel),
			logger.ErrorField("error", err))
		return fmt.Errorf("ffmpeg process failed: %w", err)
	}

	t.logger.Debug("✅ Transcoding completed successfully", logger.String("channel", channel))
	return nil
}

// StopActiveStream stops and cleans up resources for a specific channel stream.
func (t *Impl) StopActiveStream(channel string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// Check if the stream is still active
	_, streamActive := t.activeStreams[channel]
	if !streamActive {
		// Stream already stopped
		return
	}

	// Remove the active stream
	delete(t.activeStreams, channel)

	// Look for any ffmpeg processes for this channel and stop them
	if pid, exists := t.ffmpegProcesses[channel]; exists {
		t.logger.Debug("🔫 Stopping ffmpeg process",
			logger.Int("pid", pid),
			logger.String("channel", channel))
		process, err := os.FindProcess(pid)
		if err == nil {
			if killErr := process.Kill(); killErr != nil {
				// Only log error if it's not "process already finished"
				if !strings.Contains(killErr.Error(), "process already finished") &&
					!strings.Contains(killErr.Error(), "no such process") {
					t.logger.Error("❌ Error killing ffmpeg process", logger.ErrorField("error", killErr))
				}
			} else {
				t.logger.Debug("✅ Successfully killed ffmpeg process", logger.Int("pid", pid))
			}
		}
		delete(t.ffmpegProcesses, channel)
	}

	t.logger.Info("⏹️  Stream stopped",
		logger.String("channel", channel))
}

// Shutdown performs a graceful shutdown of the transcoder and all its resources.
func (t *Impl) Shutdown() {
	defer utils.TimeOperation("Shutdown transcoder")()
	t.logger.Info("🛑 Stopping transcoder gracefully")

	// Stop the activity checker
	if t.stopActivityCheck != nil {
		t.stopActivityCheck()
	}

	// Stop all processes
	t.StopAllTranscoding()
}
