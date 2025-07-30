// Package container provides dependency injection container for the HDHomeRun proxy.
package container

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/attaebra/hdhr-proxy/internal/config"
	"github.com/attaebra/hdhr-proxy/internal/interfaces"
	"github.com/attaebra/hdhr-proxy/internal/logger"

	"github.com/attaebra/hdhr-proxy/internal/media/ffmpeg"
	"github.com/attaebra/hdhr-proxy/internal/media/stream"
	"github.com/attaebra/hdhr-proxy/internal/media/transcoder"
	"github.com/attaebra/hdhr-proxy/internal/proxy"
	"github.com/attaebra/hdhr-proxy/internal/utils"
)

// Container holds all application dependencies.
type Container struct {
	config *config.Config

	// Core components
	httpClient        interfaces.HTTPClient
	streamClient      interfaces.HTTPClient
	ffmpegConfig      interfaces.FFmpegConfig
	streamHelper      interfaces.StreamHelper
	hdhrProxy         interfaces.HDHRProxy
	transcoder        interfaces.Transcoder
	securityValidator interfaces.SecurityValidator

	// HTTP servers
	apiServer   *http.Server
	mediaServer *http.Server
}

// New creates a new dependency injection container with the provided configuration.
func New(cfg *config.Config) (*Container, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	container := &Container{
		config: cfg,
	}

	// Initialize dependencies in the correct order
	if err := container.initializeHTTPClients(); err != nil {
		return nil, fmt.Errorf("failed to initialize HTTP clients: %w", err)
	}

	if err := container.initializeFFmpegConfig(); err != nil {
		return nil, fmt.Errorf("failed to initialize FFmpeg config: %w", err)
	}

	if err := container.initializeStreamHelper(); err != nil {
		return nil, fmt.Errorf("failed to initialize stream helper: %w", err)
	}

	if err := container.initializeSecurityValidator(); err != nil {
		return nil, fmt.Errorf("failed to initialize security validator: %w", err)
	}

	if err := container.initializeHDHRProxy(); err != nil {
		return nil, fmt.Errorf("failed to initialize HDHR proxy: %w", err)
	}

	if err := container.initializeTranscoder(); err != nil {
		return nil, fmt.Errorf("failed to initialize transcoder: %w", err)
	}

	if err := container.initializeServers(); err != nil {
		return nil, fmt.Errorf("failed to initialize servers: %w", err)
	}

	return container, nil
}

// initializeHTTPClients creates optimized HTTP clients.
func (c *Container) initializeHTTPClients() error {
	// Create HTTP client for API requests
	c.httpClient = utils.HTTPClient(c.config.HTTPClientTimeout)

	// Create HTTP client for streaming (no timeout)
	c.streamClient = utils.HTTPClient(c.config.StreamClientTimeout)

	logger.Debug("Initialized HTTP clients - API timeout: %v, Stream timeout: %v",
		c.config.HTTPClientTimeout, c.config.StreamClientTimeout)

	return nil
}

// initializeFFmpegConfig creates the FFmpeg configuration.
func (c *Container) initializeFFmpegConfig() error {
	cfg := ffmpeg.NewOptimizedConfig()

	// Apply configuration from config
	cfg.SetAudioBitrate(c.config.AudioBitrate)
	cfg.SetAudioChannels(c.config.AudioChannels)
	cfg.SetPreset(c.config.Preset)
	cfg.SetTune(c.config.Tune)

	c.ffmpegConfig = cfg

	logger.Debug("Initialized FFmpeg config - Bitrate: %s, Channels: %s, Preset: %s",
		c.config.AudioBitrate, c.config.AudioChannels, c.config.Preset)

	return nil
}

// initializeStreamHelper creates the stream helper.
func (c *Container) initializeStreamHelper() error {
	c.streamHelper = stream.NewHelper()

	logger.Debug("Initialized stream helper")
	return nil
}

// initializeSecurityValidator creates the security validator.
func (c *Container) initializeSecurityValidator() error {
	c.securityValidator = &utils.DefaultSecurityValidator{}

	logger.Debug("Initialized security validator")
	return nil
}

// initializeHDHRProxy creates the HDHomeRun proxy with dependencies.
func (c *Container) initializeHDHRProxy() error {
	c.hdhrProxy = proxy.NewHDHRProxyWithDependencies(
		c.config.HDHomeRunIP,
		c.httpClient,
	)

	// Fetch device ID
	if err := c.hdhrProxy.FetchDeviceID(); err != nil {
		logger.Warn("Could not fetch device ID from HDHomeRun: %v", err)
		logger.Warn("Using default device ID: %s", c.hdhrProxy.DeviceID())
	} else {
		logger.Info("Device ID: %s, Reversed: %s", c.hdhrProxy.DeviceID(), c.hdhrProxy.ReverseDeviceID())
	}

	logger.Debug("Initialized HDHR proxy for IP: %s", c.config.HDHomeRunIP)
	return nil
}

// initializeTranscoder creates the transcoder with all dependencies.
func (c *Container) initializeTranscoder() error {
	transcoderDeps := &transcoder.Dependencies{
		Config:            c.config,
		HTTPClient:        c.httpClient,
		StreamClient:      c.streamClient,
		FFmpegConfig:      c.ffmpegConfig,
		StreamHelper:      c.streamHelper,
		HDHRProxy:         c.hdhrProxy,
		SecurityValidator: c.securityValidator,
	}

	var err error
	c.transcoder, err = transcoder.Transcoder(transcoderDeps)
	if err != nil {
		return fmt.Errorf("failed to create transcoder: %w", err)
	}

	logger.Debug("Initialized transcoder with dependencies")
	return nil
}

// initializeServers creates the HTTP servers.
func (c *Container) initializeServers() error {
	// Create API server
	c.apiServer = &http.Server{
		Addr:         fmt.Sprintf("0.0.0.0:%d", c.config.APIPort),
		Handler:      c.hdhrProxy.CreateAPIHandler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Create media server
	c.mediaServer = &http.Server{
		Addr:         fmt.Sprintf("0.0.0.0:%d", c.config.MediaPort),
		Handler:      c.transcoder.CreateMediaHandler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // No write timeout for streaming
		IdleTimeout:  120 * time.Second,
	}

	logger.Debug("Initialized servers - API: %d, Media: %d", c.config.APIPort, c.config.MediaPort)
	return nil
}

// GetAPIServer returns the API server.
func (c *Container) GetAPIServer() *http.Server {
	return c.apiServer
}

// GetMediaServer returns the media server.
func (c *Container) GetMediaServer() *http.Server {
	return c.mediaServer
}

// GetTranscoder returns the transcoder instance.
func (c *Container) GetTranscoder() interfaces.Transcoder {
	return c.transcoder
}

// GetHDHRProxy returns the HDHomeRun proxy instance.
func (c *Container) GetHDHRProxy() interfaces.HDHRProxy {
	return c.hdhrProxy
}

// GetConfig returns the configuration.
func (c *Container) GetConfig() *config.Config {
	return c.config
}

// Shutdown performs graceful shutdown of all components.
func (c *Container) Shutdown(ctx context.Context) error {
	logger.Info("Shutting down container...")

	// Shutdown transcoder first to stop ongoing streams
	if c.transcoder != nil {
		c.transcoder.Shutdown()
	}

	// Shutdown servers
	if c.apiServer != nil {
		if err := c.apiServer.Shutdown(ctx); err != nil {
			logger.Error("Error shutting down API server: %v", err)
		}
	}

	if c.mediaServer != nil {
		if err := c.mediaServer.Shutdown(ctx); err != nil {
			logger.Error("Error shutting down media server: %v", err)
		}
	}

	logger.Info("Container shutdown complete")
	return nil
}
