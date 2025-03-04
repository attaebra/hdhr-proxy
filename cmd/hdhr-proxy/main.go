// Package main implements the HDHomeRun AC4 proxy service, which facilitates
// communication between Emby/Jellyfin/Plex and HDHomeRun devices.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/attaebra/hdhr-proxy/internal/logger"
	"github.com/attaebra/hdhr-proxy/internal/media"
	"github.com/attaebra/hdhr-proxy/internal/proxy"
)

func main() {
	// Parse command line arguments
	hdhrIP := flag.String("hdhr-ip", "", "IP address of the HDHomeRun device")
	appPort := flag.Int("app-port", 80, "Port for the API server")
	mediaPort := flag.Int("media-port", 5004, "Port for the media server (MUST be 5004 for client compatibility)")
	ffmpegPath := flag.String("ffmpeg", "/usr/bin/ffmpeg", "Path to the FFmpeg binary")
	logLevel := flag.String("log-level", "info", "Logging level: error, warn, info, debug")
	flag.Parse()

	// Set the logging level
	logger.SetLevel(logger.LevelFromString(*logLevel))
	logger.Info("Log level set to %s", *logLevel)

	// Check for required arguments
	if *hdhrIP == "" {
		logger.Fatal("HDHomeRun IP address is required (-hdhr-ip)")
	}

	// Initialize the proxy
	hdhrProxy := proxy.NewHDHRProxy(*hdhrIP)

	// Fetch the device ID
	err := hdhrProxy.FetchDeviceID()
	if err != nil {
		logger.Warn("Could not fetch device ID from HDHomeRun: %v", err)
		logger.Warn("Using default device ID: %s", hdhrProxy.DeviceID)
	} else {
		logger.Info("Device ID: %s, Reversed: %s", hdhrProxy.DeviceID, hdhrProxy.ReverseDeviceID())
	}

	// Initialize the transcoder
	transcoder := media.NewTranscoder(*ffmpegPath, *hdhrIP)

	// Set up API server
	apiServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", *appPort),
		Handler: hdhrProxy.CreateAPIHandler(),
	}

	// Set up media server
	mediaServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", *mediaPort),
		Handler: transcoder.CreateMediaHandler(),
	}

	// Start the API server
	go func() {
		logger.Info("Starting API server on port %d", *appPort)
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Error starting API server: %v", err)
		}
	}()

	// Start the media server
	go func() {
		logger.Info("Starting media server on port %d", *mediaPort)
		if err := mediaServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Error starting media server: %v", err)
		}
	}()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down servers...")

	// Clean up resources and exit
	transcoder.StopAllTranscoding()
	logger.Info("Bye!")
}
