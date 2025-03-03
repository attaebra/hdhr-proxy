# HDHR-AC4-Go

[![Docker Build](https://img.shields.io/github/actions/workflow/status/attaebra/hdhr-proxy/docker-build.yml?label=Docker%20Build&logo=docker)](https://github.com/attaebra/hdhr-proxy/actions/workflows/docker-build.yml)
[![Go Tests](https://img.shields.io/github/actions/workflow/status/attaebra/hdhr-proxy/go-tests.yml?label=Tests&logo=go)](https://github.com/attaebra/hdhr-proxy/actions/workflows/go-tests.yml)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

A high-performance Go implementation of a proxy for HDHomeRun ATSC 3.0 tuners that converts AC4 audio to EAC3 for compatibility with media players like Plex, Emby, and VLC.

![HDHomeRun FLEX 4K](https://www.silicondust.com/wordpress/wp-content/uploads/2021/06/HDHR-FLEX-4K-1000x563.jpg)

## Overview

ATSC 3.0 (NextGen TV) broadcasts often use AC4 audio encoding, which isn't compatible with many media players. This proxy sits between your media player and an HDHomeRun ATSC 3.0 tuner to:

1. Transparently proxy HDHomeRun API requests
2. On-the-fly transcode AC4 audio to EAC3 audio using FFmpeg
3. Modify API responses to ensure compatibility with media players
4. Provide a seamless viewing experience

## Features

- **High Performance**: Written in Go for efficient, concurrent handling of streams
- **Transparent Proxying**: Full HDHomeRun API compatibility
- **Automatic Device Detection**: Detects and reverses the device ID to avoid conflicts
- **Dynamic Channel Handling**: Works with all available channels
- **Cross-Platform Support**: Runs on x86_64 and ARM64 architectures
- **Docker Ready**: Easy deployment via containerization
- **Minimal Dependencies**: Uses FFmpeg extracted from Emby for transcoding

## Project Structure

The codebase is organized as follows:

```
hdhr-ac4/
├── cmd/                 # Application entry points
│   └── hdhr-proxy/      # Main application
├── internal/            # Private application packages
│   ├── logger/          # Logging functionality
│   ├── media/           # Media transcoding and streaming
│   └── proxy/           # HDHomeRun API proxying
├── pkg/                 # Public API packages (if any)
├── .github/             # GitHub Actions workflows
├── Dockerfile           # Docker build instructions
└── test-local.sh        # Local testing script
```

- **cmd/hdhr-proxy/**: Contains the main application entry point
- **internal/logger/**: Implements the custom logging system with multiple log levels
- **internal/media/**: Handles media stream transcoding and the `/status` endpoint
- **internal/proxy/**: Manages the HDHomeRun device communication and API transformations

## Quick Start

### Using Docker

```bash
docker run -p 5003:80 -p 5004:5004 -e HDHR_IP=192.168.0.123 ghcr.io/attaebra/hdhr-ac4-go:latest
```

Replace `192.168.0.123` with the IP address of your HDHomeRun device.

### Environment Variables

- `HDHR_IP` (required): IP address of your HDHomeRun tuner
- `LINK` (optional): Custom URL to an Emby .deb package for FFmpeg extraction
- `LOG_LEVEL` (optional): Logging verbosity (debug, info, warn, error) - default is info

### Port Mapping

- Port 80: HDHomeRun API proxy (can be mapped to any host port)
- Port 5004: Media streaming (must be mapped to host port 5004)

## Connecting to Media Players

### Plex

1. In Plex, go to Settings → Live TV → Set up Live TV
2. Add a new tuner device
3. Enter the container's IP and port (e.g., `192.168.0.234:5003`)
4. Follow the on-screen instructions to complete setup

### VLC

1. Open VLC and select "Open Network Stream"
2. Enter the URL: `http://localhost:5004/auto/v5.1` (replace v5.1 with your channel number)
3. Click "Play"

## Development

### Prerequisites

- Go 1.24 or later
- Docker (for building and testing containers)

### Building From Source

1. Clone the repository
   ```bash
   git clone https://github.com/yourusername/hdhr-ac4-go.git
   cd hdhr-ac4-go
   ```

2. Run the tests
   ```bash
   go test ./...
   ```

3. Build the application
   ```bash
   go build ./cmd/hdhr-proxy
   ```

### Testing

The project includes a comprehensive test suite to validate functionality:

#### Running Tests

Run all tests:
```bash
go test ./...
```

Run tests with verbose output:
```bash
go test -v ./...
```