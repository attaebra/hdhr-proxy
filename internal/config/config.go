// Package config provides centralized configuration management for the HDHomeRun proxy.
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/attaebra/hdhr-proxy/internal/constants"
)

// Config holds the application configuration.
type Config struct {
	// Server configuration
	APIPort   int
	MediaPort int

	// HDHomeRun configuration
	HDHomeRunIP string

	// FFmpeg configuration
	FFmpegPath string
	BufferSize string

	// HTTP client timeouts
	HTTPClientTimeout   time.Duration
	StreamClientTimeout time.Duration

	// Activity monitoring
	ActivityCheckInterval time.Duration
	MaxInactivityDuration time.Duration

	// Runtime configuration
	LogLevel string
	Debug    bool
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		// Server defaults
		APIPort:   constants.DefaultAPIPort,
		MediaPort: constants.DefaultMediaPort,
		LogLevel:  "info",

		// FFmpeg defaults
		FFmpegPath: "/usr/bin/ffmpeg",

		// HTTP Client defaults
		HTTPClientTimeout:   30 * time.Second,
		StreamClientTimeout: 0, // No timeout for streaming

		// Stream defaults
		ActivityCheckInterval: 30 * time.Second,
		MaxInactivityDuration: 2 * time.Minute,

		// FFmpeg defaults
		BufferSize: "2048k",
	}
}

// LoadFromEnvironment loads configuration from environment variables and command line flags.
func (c *Config) LoadFromEnvironment() {
	// Load from environment variables
	if hdhrIP := os.Getenv("HDHR_IP"); hdhrIP != "" {
		c.HDHomeRunIP = hdhrIP
	}

	if logLevel := os.Getenv("LOG_LEVEL"); logLevel != "" {
		c.LogLevel = logLevel
	}

	if ffmpegPath := os.Getenv("FFMPEG_PATH"); ffmpegPath != "" {
		c.FFmpegPath = ffmpegPath
	}

	// HTTP client settings are now handled directly in utils/http.go
}

// LoadFromFlags loads configuration from command line flags.
func (c *Config) LoadFromFlags(hdhrIP *string, appPort *int, mediaPort *int, ffmpegPath *string, logLevel *string) {
	if hdhrIP != nil && *hdhrIP != "" {
		c.HDHomeRunIP = *hdhrIP
	}

	if appPort != nil {
		c.APIPort = *appPort
	}

	if mediaPort != nil {
		c.MediaPort = *mediaPort
	}

	if ffmpegPath != nil && *ffmpegPath != "" {
		c.FFmpegPath = *ffmpegPath
	}

	if logLevel != nil && *logLevel != "" {
		c.LogLevel = *logLevel
	}
}

// Validate ensures the configuration is valid.
func (c *Config) Validate() error {
	if c.HDHomeRunIP == "" {
		return fmt.Errorf("HDHomeRun IP address is required")
	}

	if c.APIPort <= 0 || c.APIPort > 65535 {
		return fmt.Errorf("invalid API port: %d", c.APIPort)
	}

	if c.MediaPort <= 0 || c.MediaPort > 65535 {
		return fmt.Errorf("invalid media port: %d", c.MediaPort)
	}

	if c.FFmpegPath == "" {
		return fmt.Errorf("FFmpeg path is required")
	}

	return nil
}
