// Package main implements the HDHomeRun AC4 proxy service, which facilitates
// communication between Emby/Jellyfin/Plex and HDHomeRun devices.
package main

import (
	"context"
	"flag"
	"fmt"
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

// Build-time variables, set via -ldflags during build.
var (
	Version   = "dev"     // Set via -ldflags "-X main.Version=v1.0.0"
	BuildTime = "unknown" // Set via -ldflags "-X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	GitCommit = "unknown" // Set via -ldflags "-X main.GitCommit=$(git rev-parse --short HEAD)"
)

func main() {
	// Parse command line arguments.
	hdhrIP := flag.String("hdhr-ip", "", "IP address of the HDHomeRun device")
	appPort := flag.Int("app-port", constants.DefaultAPIPort, "Port for the API server")
	mediaPort := flag.Int("media-port", constants.DefaultMediaPort, "Port for the media server (MUST be 5004 for client compatibility)")
	ffmpegPath := flag.String("ffmpeg", "/usr/bin/ffmpeg", "Path to the FFmpeg binary")
	logLevel := flag.String("log-level", "info", "Logging level: error, warn, info, debug")
	showVersion := flag.Bool("version", false, "Show version information and exit")
	flag.Parse()

	// Handle version flag
	if *showVersion {
		fmt.Printf("HDHR Proxy %s\n", Version)
		fmt.Printf("Build time: %s\n", BuildTime)
		fmt.Printf("Git commit: %s\n", GitCommit)
		return
	}

	// Create configuration with defaults
	cfg := config.DefaultConfig()

	// Load configuration from command line flags
	cfg.LoadFromFlags(hdhrIP, appPort, mediaPort, ffmpegPath, logLevel)

	// Load configuration from environment variables
	cfg.LoadFromEnvironment()

	// Set the logging level and initialize structured logger
	logger.SetLevel(logger.LevelFromString(cfg.LogLevel))

	// Create a beautiful startup banner
	logger.Info("🎯 HDHR Proxy Starting",
		logger.String("version", Version),
		logger.String("build_time", BuildTime),
		logger.String("git_commit", GitCommit),
		logger.String("log_level", cfg.LogLevel))

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		logger.Fatal("❌ Configuration validation failed", logger.ErrorField("error", err))
	}

	// Initialize dependency injection container
	container, err := container.Initialize(cfg)
	if err != nil {
		logger.Fatal("❌ Failed to initialize container", logger.ErrorField("error", err))
	}

	// Show beautiful configuration summary
	logger.Info("⚙️  Configuration loaded",
		logger.String("hdhr_ip", cfg.HDHomeRunIP),
		logger.Int("api_port", cfg.APIPort),
		logger.Int("media_port", cfg.MediaPort),
		logger.String("ffmpeg_path", cfg.FFmpegPath))

	// Get servers from container
	apiServer := container.GetAPIServer()
	mediaServer := container.GetMediaServer()

	// Start the API server.
	go func() {
		logger.Info("🌐 Starting API server", logger.Int("port", cfg.APIPort))
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("❌ Error starting API server", logger.ErrorField("error", err))
		}
	}()

	// Start the media server.
	go func() {
		logger.Info("📺 Starting media server", logger.Int("port", cfg.MediaPort))
		if err := mediaServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("❌ Error starting media server", logger.ErrorField("error", err))
		}
	}()

	// Create a context for graceful shutdown.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Set up signal handling for graceful shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("🛑 Graceful shutdown initiated...")

	// Gracefully shut down all components
	if err := container.Shutdown(shutdownCtx); err != nil {
		logger.Error("❌ Error during shutdown", logger.ErrorField("error", err))
	}

	logger.Info("👋 HDHR Proxy shutdown complete - Goodbye!")
}
