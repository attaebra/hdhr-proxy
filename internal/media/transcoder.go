package media

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
)

// Transcoder manages the FFmpeg process for transcoding AC4 to AC3
type Transcoder struct {
	FFmpegPath string
	InputURL   string
	ctx        context.Context
	cancel     context.CancelFunc
	cmd        *exec.Cmd
	mutex      sync.Mutex
}

// NewTranscoder creates a new transcoder instance
func NewTranscoder(ffmpegPath, hdhrIP string) *Transcoder {
	if ffmpegPath == "" {
		ffmpegPath = "/usr/bin/ffmpeg" // Default path
	}

	return &Transcoder{
		FFmpegPath: ffmpegPath,
		InputURL:   fmt.Sprintf("http://%s:5004", hdhrIP),
	}
}

// TranscodeChannel starts the ffmpeg process to transcode from AC4 to AC3
func (t *Transcoder) TranscodeChannel(w http.ResponseWriter, channel string) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// Create a cancelable context
	t.ctx, t.cancel = context.WithCancel(context.Background())
	
	// Create an HTTP client to fetch the stream
	client := &http.Client{}
	
	// Create the request with context
	req, err := http.NewRequestWithContext(t.ctx, "GET", fmt.Sprintf("%s/auto/%s", t.InputURL, channel), nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}
	
	// Execute the request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch stream: %w", err)
	}
	defer resp.Body.Close()
	
	// Check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("invalid response from HDHomeRun: %d", resp.StatusCode)
	}
	
	// Set up ffmpeg command
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
	
	// Get pipes for stdin and stdout
	stdin, err := t.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}
	
	stdout, err := t.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	
	stderr, err := t.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}
	
	// Start the ffmpeg process
	err = t.cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}
	
	// Set up cleanup function
	cleanup := func() {
		if t.cancel != nil {
			t.cancel()
		}
		
		if t.cmd != nil && t.cmd.Process != nil {
			t.cmd.Process.Kill()
		}
		
		// Wait for process to exit to avoid zombie processes
		if t.cmd != nil {
			t.cmd.Wait()
		}
	}
	
	// Handle client disconnection
	go func() {
		<-t.ctx.Done()
		cleanup()
	}()
	
	// Log stderr output from ffmpeg
	go func() {
		io.Copy(os.Stderr, stderr)
	}()
	
	// Copy from HDHomeRun to ffmpeg stdin
	go func() {
		defer stdin.Close()
		io.Copy(stdin, resp.Body)
	}()
	
	// Copy from ffmpeg stdout to response
	_, err = io.Copy(w, stdout)
	if err != nil && err != io.ErrClosedPipe && t.ctx.Err() == nil {
		return fmt.Errorf("error writing to response: %w", err)
	}
	
	return nil
}

// Stop stops the transcoding process
func (t *Transcoder) Stop() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	
	if t.cancel != nil {
		t.cancel()
	}
} 