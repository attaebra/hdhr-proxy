package media

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/attaebra/hdhr-proxy/internal/logger"
)

// Transcoder manages the FFmpeg process for transcoding AC4 to EAC3
type Transcoder struct {
	FFmpegPath    string
	InputURL      string
	ctx           context.Context
	cancel        context.CancelFunc
	cmd           *exec.Cmd
	mutex         sync.Mutex
	activeStreams map[string]time.Time // Track active streams by channel ID
}

// NewTranscoder creates a new transcoder instance
func NewTranscoder(ffmpegPath, hdhrIP string) *Transcoder {
	if ffmpegPath == "" {
		ffmpegPath = "/usr/bin/ffmpeg" // Default path
	}

	logger.Info("Creating new transcoder with ffmpeg path: %s and HDHomeRun IP: %s", ffmpegPath, hdhrIP)

	return &Transcoder{
		FFmpegPath:    ffmpegPath,
		InputURL:      fmt.Sprintf("http://%s:5004", hdhrIP),
		activeStreams: make(map[string]time.Time),
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

	t.mutex.Lock()

	// Create a cancelable context
	t.ctx, t.cancel = context.WithCancel(context.Background())

	// Create a cleanup function that will be called on any exit path
	cleanup := func() {
		logger.Debug("Cleaning up transcoding resources for channel %s...", channel)
		if t.cancel != nil {
			t.cancel()
		}

		if t.cmd != nil && t.cmd.Process != nil {
			pid := t.cmd.Process.Pid
			logger.Debug("Killing ffmpeg process with PID: %d", pid)
			killErr := t.cmd.Process.Kill()
			if killErr != nil {
				logger.Error("Error killing ffmpeg process (PID: %d): %v", pid, killErr)
			}
		}

		// Wait for process to exit to avoid zombie processes
		if t.cmd != nil {
			waitErr := t.cmd.Wait()
			if waitErr != nil && !strings.Contains(waitErr.Error(), "already released") &&
				!strings.Contains(waitErr.Error(), "already finished") {
				logger.Debug("Wait error for ffmpeg process: %v", waitErr)
			}
		}
		logger.Debug("Cleanup completed for channel %s", channel)
	}

	// Make sure we clean up on return
	defer cleanup()
	t.mutex.Unlock()

	// Create an HTTP client to fetch the stream
	client := &http.Client{
		Timeout: 10 * time.Second, // Set connection timeout
	}

	// Create the request with context
	sourceURL := fmt.Sprintf("%s/auto/v%s", t.InputURL, channel)
	logger.Debug("Connecting to source URL: %s", sourceURL)
	req, err := http.NewRequestWithContext(t.ctx, "GET", sourceURL, nil)
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
	logger.Debug("Connected to HDHomeRun in %.2f ms", time.Since(connStart).Milliseconds())

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

	// Set up ffmpeg command
	logger.Debug("Setting up ffmpeg command with path: %s", t.FFmpegPath)

	t.mutex.Lock()
	t.cmd = exec.CommandContext(t.ctx, t.FFmpegPath,
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
	t.mutex.Unlock()

	// Get pipes for stdin and stdout
	stdin, err := t.cmd.StdinPipe()
	if err != nil {
		logger.Error("Failed to get stdin pipe: %v", err)
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := t.cmd.StdoutPipe()
	if err != nil {
		logger.Error("Failed to get stdout pipe: %v", err)
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := t.cmd.StderrPipe()
	if err != nil {
		logger.Error("Failed to get stderr pipe: %v", err)
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	// Start the ffmpeg process
	logger.Debug("Starting ffmpeg process...")
	ffmpegStart := time.Now()

	t.mutex.Lock()
	err = t.cmd.Start()
	t.mutex.Unlock()

	if err != nil {
		logger.Error("Failed to start ffmpeg: %v", err)
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	t.mutex.Lock()
	ffmpegPid := t.cmd.Process.Pid
	t.mutex.Unlock()

	logger.Debug("ffmpeg process started successfully with PID: %d in %.2f ms",
		ffmpegPid, time.Since(ffmpegStart).Milliseconds())

	// Handle client disconnection
	disconnected := make(chan struct{})

	// Log stderr output from ffmpeg
	go func() {
		logger.Debug("Starting stderr monitoring for channel %s (PID: %d)...", channel, ffmpegPid)
		// Buffer to capture errors
		buffer := make([]byte, 4096)

		for {
			n, err := stderr.Read(buffer)
			if err != nil {
				if err != io.EOF && t.ctx.Err() == nil {
					logger.Debug("stderr read error: %v", err)
				}
				break
			}

			if n > 0 {
				output := string(buffer[:n])
				logger.Debug("ffmpeg[%d]: %s", ffmpegPid, strings.TrimSpace(output))
			}
		}
		logger.Debug("stderr monitoring ended for PID: %d", ffmpegPid)
	}()

	// Copy from HDHomeRun to ffmpeg stdin
	go func() {
		logger.Debug("Starting stream copy from HDHomeRun to ffmpeg for channel %s...", channel)
		defer stdin.Close()

		buffer := make([]byte, 64*1024) // Use a 64KB buffer
		var totalBytes int64

		// Periodically log progress
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		done := make(chan struct{})

		// Copy in chunks and report progress
		go func() {
			defer close(done)

			for {
				nr, er := resp.Body.Read(buffer)
				if nr > 0 {
					nw, ew := stdin.Write(buffer[0:nr])
					if nw < 0 || nr < nw {
						nw = 0
					}
					totalBytes += int64(nw)

					if ew != nil {
						if t.ctx.Err() == nil { // Only log if not cancelled
							if strings.Contains(ew.Error(), "broken pipe") {
								logger.Info("Client disconnected while writing to ffmpeg (broken pipe)")
							} else {
								logger.Error("Error writing to ffmpeg: %v", ew)
							}
						}
						break
					}
				}
				if er != nil {
					if er != io.EOF {
						if t.ctx.Err() == nil { // Only log if not cancelled
							if strings.Contains(er.Error(), "connection reset") ||
								strings.Contains(er.Error(), "closed") {
								logger.Info("HDHomeRun connection closed or reset")
							} else {
								logger.Error("Error reading from HDHomeRun: %v", er)
							}
						}
					}
					break
				}
			}
		}()

		// Monitor and log progress
		for {
			select {
			case <-ticker.C:
				logger.Debug("Channel %s: Copied %d bytes from HDHomeRun to ffmpeg so far",
					channel, totalBytes)
			case <-done:
				logger.Debug("Channel %s: Finished copying from HDHomeRun, total bytes: %d",
					channel, totalBytes)
				close(disconnected)
				return
			case <-t.ctx.Done():
				logger.Debug("Channel %s: Context cancelled while copying from HDHomeRun", channel)
				return
			}
		}
	}()

	// Copy from ffmpeg stdout to response
	logger.Debug("Starting stream copy from ffmpeg to response writer for channel %s...", channel)

	// Set appropriate headers for the response
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("X-Transcoded-By", "hdhr-proxy")

	// Use a buffer for better performance
	buffer := make([]byte, 128*1024) // 128KB buffer
	var written int64

	// Copy in chunks and count bytes
	for {
		nr, er := stdout.Read(buffer)
		if nr > 0 {
			nw, ew := w.Write(buffer[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
			}
			written += int64(nw)

			if ew != nil {
				if t.ctx.Err() == nil { // Only log if not cancelled
					if strings.Contains(ew.Error(), "broken pipe") ||
						strings.Contains(ew.Error(), "connection reset") {
						logger.Info("Client disconnected while writing response (channel %s)", channel)
					} else {
						logger.Error("Error writing to response for channel %s: %v", channel, ew)
					}
				}
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				if t.ctx.Err() == nil { // Only log if not cancelled
					if strings.Contains(er.Error(), "broken pipe") {
						logger.Info("ffmpeg pipe closed while reading")
					} else {
						logger.Error("Error reading from ffmpeg for channel %s: %v", channel, er)
					}
				}
			}
			break
		}
	}

	logger.Debug("Copy from ffmpeg to response ended after %d bytes for channel %s", written, channel)

	// Signal disconnection in case it hasn't been signaled yet
	select {
	case <-disconnected:
		// Already closed
	default:
		close(disconnected)
	}

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
