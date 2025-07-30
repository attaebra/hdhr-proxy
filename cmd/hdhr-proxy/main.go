// Package main implements the HDHomeRun AC4 proxy service, which facilitates
// communication between Emby/Jellyfin/Plex and HDHomeRun devices.
package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/attaebra/hdhr-proxy/internal/config"
	"github.com/attaebra/hdhr-proxy/internal/constants"
	"github.com/attaebra/hdhr-proxy/internal/container"
	"github.com/attaebra/hdhr-proxy/internal/logger"
)

func main() {
	// Parse command line arguments.
	hdhrIP := flag.String("hdhr-ip", "", "IP address of the HDHomeRun device")
	appPort := flag.Int("app-port", constants.DefaultAPIPort, "Port for the API server")
	mediaPort := flag.Int("media-port", constants.DefaultMediaPort, "Port for the media server (MUST be 5004 for client compatibility)")
	ffmpegPath := flag.String("ffmpeg", "/usr/bin/ffmpeg", "Path to the FFmpeg binary")
	logLevel := flag.String("log-level", "info", "Logging level: error, warn, info, debug")
	flag.Parse()

	// Create configuration with defaults
	cfg := config.DefaultConfig()

	// Load configuration from command line flags
	cfg.LoadFromFlags(hdhrIP, appPort, mediaPort, ffmpegPath, logLevel)

	// Load configuration from environment variables
	cfg.LoadFromEnvironment()

	// Set the logging level.
	logger.SetLevel(logger.LevelFromString(cfg.LogLevel))
	logger.Info("Log level set to %s", cfg.LogLevel)

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		logger.Fatal("Configuration validation failed: %v", err)
	}

	// Initialize dependency injection container
	container, err := container.Initialize(cfg)
	if err != nil {
		logger.Error("Failed to initialize container: %v", err)
		return
	}

	logger.Info("Configuration loaded:")
	logger.Info("  HDHomeRun IP: %s", cfg.HDHomeRunIP)
	logger.Info("  API Port: %d", cfg.APIPort)
	logger.Info("  Media Port: %d", cfg.MediaPort)
	logger.Info("  FFmpeg Path: %s", cfg.FFmpegPath)

	// Get servers from container
	apiServer := container.GetAPIServer()
	mediaServer := container.GetMediaServer()

	// Start the API server.
	go func() {
		logger.Info("Starting API server on port %d", cfg.APIPort)
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Error starting API server: %v", err)
		}
	}()

	// Start the media server.
	go func() {
		logger.Info("Starting media server on port %d", cfg.MediaPort)
		if err := mediaServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Error starting media server: %v", err)
		}
	}()

	// Create a context for graceful shutdown.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Set up signal handling for graceful shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down servers...")

	// Gracefully shut down all components
	if err := container.Shutdown(shutdownCtx); err != nil {
		logger.Error("Error during shutdown: %v", err)
	}

	logger.Info("Bye!")
}
