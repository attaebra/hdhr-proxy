// Package container implements dependency injection for the HDHomeRun proxy.
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

// Container manages all application dependencies.
type Container struct {
	config            *config.Config
	httpClient        interfaces.Client
	streamClient      interfaces.Client
	streamer          interfaces.Streamer
	ffmpegConfig      interfaces.Config
	securityValidator interfaces.SecurityValidator
	hdhrProxy         interfaces.Proxy
	transcoder        interfaces.Transcoder

	// HTTP servers
	apiServer   *http.Server
	mediaServer *http.Server
}

// Initialize sets up all dependencies using dependency injection.
func Initialize(cfg *config.Config) (*Container, error) {
	logger.Info("Initializing application container with dependency injection")

	container := &Container{
		config: cfg,
	}

	// Initialize dependencies in order
	if err := container.initializeHTTPClients(); err != nil {
		return nil, fmt.Errorf("failed to initialize HTTP clients: %w", err)
	}

	if err := container.initializeFFmpegConfig(); err != nil {
		return nil, fmt.Errorf("failed to initialize FFmpeg config: %w", err)
	}

	if err := container.initializeSecurityValidator(); err != nil {
		return nil, fmt.Errorf("failed to initialize security validator: %w", err)
	}

	if err := container.initializeStreamer(); err != nil {
		return nil, fmt.Errorf("failed to initialize stream helper: %w", err)
	}

	if err := container.initializeProxy(); err != nil {
		return nil, fmt.Errorf("failed to initialize proxy: %w", err)
	}

	if err := container.initializeTranscoder(); err != nil {
		return nil, fmt.Errorf("failed to initialize transcoder: %w", err)
	}

	if err := container.initializeServers(); err != nil {
		return nil, fmt.Errorf("failed to initialize servers: %w", err)
	}

	logger.Info("Container initialization completed successfully")
	return container, nil
}

// initializeHTTPClients creates HTTP clients.
func (c *Container) initializeHTTPClients() error {
	// Client for API requests with timeout
	c.httpClient = utils.HTTPClient(c.config.HTTPClientTimeout)

	// Client for streaming (no timeout)
	c.streamClient = utils.HTTPClient(c.config.StreamClientTimeout)

	logger.Debug("Initialized HTTP clients - API timeout: %v, Stream timeout: %v",
		c.config.HTTPClientTimeout, c.config.StreamClientTimeout)
	return nil
}

// initializeFFmpegConfig creates the FFmpeg configuration.
func (c *Container) initializeFFmpegConfig() error {
	// Use FFmpeg configuration with built-in AC4 error resilience
	c.ffmpegConfig = ffmpeg.New()

	logger.Debug("Initialized FFmpeg config with AC4 error resilience")
	return nil
}

// initializeSecurityValidator creates the security validator.
func (c *Container) initializeSecurityValidator() error {
	// Use a factory function for consistency with other dependencies
	c.securityValidator = utils.NewSecurityValidator()

	logger.Debug("Initialized security validator")
	return nil
}

// initializeStreamer creates the stream helper.
func (c *Container) initializeStreamer() error {
	c.streamer = stream.NewHelper()

	logger.Debug("Initialized stream helper")
	return nil
}

// initializeProxy creates the HDHomeRun proxy with dependency injection.
func (c *Container) initializeProxy() error {
	logger.Debug("Creating HDHomeRun proxy with injected HTTP client")

	// Use dependency injection for the proxy
	c.hdhrProxy = proxy.New(
		c.config.HDHomeRunIP,
		c.httpClient,
	)

	// Fetch the device ID from the HDHomeRun
	if err := c.hdhrProxy.FetchDeviceID(); err != nil {
		return fmt.Errorf("failed to fetch HDHomeRun device ID: %w", err)
	}

	logger.Info("HDHomeRun proxy initialized with device ID: %s", c.hdhrProxy.DeviceID())
	return nil
}

// initializeTranscoder creates the media transcoder with dependency injection.
func (c *Container) initializeTranscoder() error {
	logger.Debug("Creating transcoder with dependency injection")

	// Create transcoder dependencies struct
	deps := &transcoder.Dependencies{
		Config:            c.config,
		HTTPClient:        c.httpClient,
		StreamClient:      c.streamClient,
		FFmpegConfig:      c.ffmpegConfig,
		StreamHelper:      c.streamer,
		HDHRProxy:         c.hdhrProxy,
		SecurityValidator: c.securityValidator,
	}

	// Create transcoder with dependency injection
	var err error
	c.transcoder, err = transcoder.Transcoder(deps)
	if err != nil {
		return fmt.Errorf("failed to create transcoder: %w", err)
	}

	logger.Debug("Transcoder initialized with dependency injection")
	return nil
}

// initializeServers creates the HTTP servers.
func (c *Container) initializeServers() error {
	// Create API server
	c.apiServer = &http.Server{
		Addr:         fmt.Sprintf("0.0.0.0:%d", c.config.APIPort),
		Handler:      c.hdhrProxy.APIHandler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Create media server
	c.mediaServer = &http.Server{
		Addr:         fmt.Sprintf("0.0.0.0:%d", c.config.MediaPort),
		Handler:      c.transcoder.MediaHandler(),
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
