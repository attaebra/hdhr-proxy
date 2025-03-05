package media

import (
	"bufio"
	"context"
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

	return &Transcoder{
		FFmpegPath:     ffmpegPath,
		InputURL:       fmt.Sprintf("http://%s:5004", hdhrIP),
		ctx:            ctx,
		cancel:         cancel,
		activeStreams:  make(map[string]time.Time),
		RequestTimeout: requestTimeout,
	}
}

// TranscodeChannel starts the ffmpeg process to transcode from AC4 to EAC3
func (t *Transcoder) TranscodeChannel(w http.ResponseWriter, channel string) error {
	start := time.Now()

	// Track this stream in our active streams
	t.mutex.Lock()
	t.activeStreams[channel] = start
	activeCount := len(t.activeStreams)
	t.mutex.Unlock()

	logger.Info("Starting transcoding for channel: %s (active streams: %d)", channel, activeCount)
	logger.Debug("Using input URL: %s/auto/v%s", t.InputURL, channel)

	defer func() {
		if r := recover(); r != nil {
			logger.Error("Recovered from panic in TranscodeChannel: %v\nStack: %s", r, debug.Stack())
		}

		// Remove this stream from active streams
		t.mutex.Lock()
		delete(t.activeStreams, channel)
		duration := time.Since(start).Seconds()
		t.mutex.Unlock()

		logger.Info("Transcoding session for channel %s ended after %.2f seconds", channel, duration)
	}()

	// Create a cancelable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
		return fmt.Errorf("failed to fetch stream: %w", err)
	}
	logger.Debug("Connected to HDHomeRun in %d ms", time.Since(connStart).Milliseconds())

	defer resp.Body.Close()

	// Check response status
	logger.Debug("Received response with status: %d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		logger.Error("Invalid response from HDHomeRun: %d", resp.StatusCode)
		return fmt.Errorf("invalid response from HDHomeRun: %d", resp.StatusCode)
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
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logger.Error("Failed to get stdout pipe: %v", err)
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		logger.Error("Failed to get stderr pipe: %v", err)
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	// Start the ffmpeg process
	logger.Debug("Starting ffmpeg process...")
	ffmpegStart := time.Now()

	err = cmd.Start()
	if err != nil {
		logger.Error("Failed to start ffmpeg: %v", err)
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
			logger.Error("Error copying from HDHomeRun to ffmpeg: %v", err)
		}
		logger.Debug("Finished copying from HDHomeRun to ffmpeg, bytes copied: %d", copied)
	}()

	// Copy from ffmpeg stdout to response
	go func() {
		defer wg.Done()
		logger.Debug("Starting stream copy from ffmpeg to response for channel %s...", channel)

		copied, err := io.Copy(w, stdout)
		if err != nil && err != io.EOF && ctx.Err() == nil {
			logger.Error("Error copying from ffmpeg to response: %v", err)
		}
		logger.Debug("Finished copying from ffmpeg to response, bytes copied: %d", copied)
	}()

	// Wait for both copy operations to complete
	wg.Wait()

	// Wait for the process to exit
	err = cmd.Wait()
	if err != nil && ctx.Err() == nil {
		logger.Error("ffmpeg process exited with error: %v", err)
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

	// Handle streaming requests - the pattern is typically /auto/vX.X where X.X is the channel number
	mux.HandleFunc("/auto/", func(w http.ResponseWriter, r *http.Request) {
		remoteAddr := r.RemoteAddr
		userAgent := r.UserAgent()

		logger.Info("Received media request: %s %s from %s (User-Agent: %s)",
			r.Method, r.URL.Path, remoteAddr, userAgent)

		if !strings.HasPrefix(r.URL.Path, "/auto/v") {
			logger.Debug("Path %s doesn't match /auto/v pattern, returning 404", r.URL.Path)
			http.NotFound(w, r)
			return
		}

		channel := strings.TrimPrefix(r.URL.Path, "/auto/v")
		if channel == "" {
			logger.Debug("Empty channel after prefix trim, returning 404")
			http.NotFound(w, r)
			return
		}

		logger.Debug("Starting transcoding for channel: %s from client %s", channel, remoteAddr)
		err := t.TranscodeChannel(w, channel)
		if err != nil {
			logger.Error("Transcoding error for channel %s: %v", channel, err)
			http.Error(w, "Transcoding error: "+err.Error(), http.StatusInternalServerError)
		}
		logger.Debug("Transcoding handler completed for channel: %s from client %s", channel, remoteAddr)
	})

	// Add status endpoint for active streams
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		t.mutex.Lock()
		activeStreams := len(t.activeStreams)
		streams := make(map[string]float64)

		for channel, startTime := range t.activeStreams {
			streams[channel] = time.Since(startTime).Seconds()
		}
		t.mutex.Unlock()

		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "HDHomeRun AC4 Proxy Status\n")
		fmt.Fprintf(w, "=========================\n")
		fmt.Fprintf(w, "Active Streams: %d\n\n", activeStreams)

		if activeStreams > 0 {
			fmt.Fprintf(w, "Channel    Duration (s)\n")
			fmt.Fprintf(w, "--------------------\n")
			for channel, duration := range streams {
				fmt.Fprintf(w, "%-10s %.2f\n", channel, duration)
			}
		}

		logger.Info("Status request from %s (active streams: %d)", r.RemoteAddr, activeStreams)
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
