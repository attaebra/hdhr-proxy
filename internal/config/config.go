// Package config provides centralized configuration management for the HDHomeRun proxy.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/attaebra/hdhr-proxy/internal/constants"
	"github.com/attaebra/hdhr-proxy/internal/logger"
)

// Config holds all application configuration.
type Config struct {
	// Server Configuration
	HDHomeRunIP string
	APIPort     int
	MediaPort   int
	LogLevel    string

	// FFmpeg Configuration
	FFmpegPath string

	// HTTP Client Configuration
	HTTPClientTimeout   time.Duration
	StreamClientTimeout time.Duration
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	MaxConnsPerHost     int
	IdleConnTimeout     time.Duration

	// Stream Configuration
	RequestTimeout        time.Duration
	ActivityCheckInterval time.Duration
	MaxInactivityDuration time.Duration
	PreBufferTimeout      time.Duration
	MinBufferThreshold    int

	// FFmpeg Configuration
	AudioBitrate       string
	AudioChannels      string
	BufferSize         string
	MaxRate            string
	Preset             string
	Tune               string
	ThreadQueueSize    string
	MaxMuxingQueueSize string
	Threads            string
	Format             string
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
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     50,
		IdleConnTimeout:     90 * time.Second,

		// Stream defaults
		RequestTimeout:        0, // No timeout by default
		ActivityCheckInterval: 30 * time.Second,
		MaxInactivityDuration: 2 * time.Minute,
		PreBufferTimeout:      20 * time.Millisecond,
		MinBufferThreshold:    32 * 1024, // 32KB

		// FFmpeg defaults
		AudioBitrate:       "384k",
		AudioChannels:      "2",
		BufferSize:         "2048k",
		MaxRate:            "30M",
		Preset:             "superfast",
		Tune:               "zerolatency",
		ThreadQueueSize:    "512",
		MaxMuxingQueueSize: "256",
		Threads:            "4",
		Format:             "mpegts",
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

	// Parse REQUEST_TIMEOUT
	if timeoutStr := os.Getenv("REQUEST_TIMEOUT"); timeoutStr != "" {
		if timeout, err := time.ParseDuration(timeoutStr); err == nil {
			c.RequestTimeout = timeout
			logger.Debug("Using custom request timeout: %s", timeout)
		} else {
			logger.Warn("Invalid REQUEST_TIMEOUT format, using default: %v", err)
		}
	}

	// Parse HTTP client settings
	if maxConns := getEnvInt("MAX_IDLE_CONNS", 0); maxConns > 0 {
		c.MaxIdleConns = maxConns
	}

	if maxConnsPerHost := getEnvInt("MAX_IDLE_CONNS_PER_HOST", 0); maxConnsPerHost > 0 {
		c.MaxIdleConnsPerHost = maxConnsPerHost
	}
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

// getEnvInt gets an integer from environment variable with default value.
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
