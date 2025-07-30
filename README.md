# HDHR Proxy

[![Docker Build](https://img.shields.io/github/actions/workflow/status/attaebra/hdhr-proxy/docker-build.yml?label=Docker%20Build&logo=docker)](https://github.com/attaebra/hdhr-proxy/actions/workflows/docker-build.yml)
[![Go Tests](https://img.shields.io/github/actions/workflow/status/attaebra/hdhr-proxy/go-tests.yml?label=Tests&logo=go)](https://github.com/attaebra/hdhr-proxy/actions/workflows/go-tests.yml)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

A **high-performance Go proxy** for HDHomeRun ATSC 3.0 tuners that seamlessly converts AC4 audio to EAC3, enabling full compatibility with media players like Plex, Emby, and VLC.

## Overview

ATSC 3.0 (NextGen TV) broadcasts use AC4 audio encoding, which many media players don't support. This proxy provides transparent AC4→EAC3 transcoding while maintaining the full HDHomeRun API experience.

**🎯 Core Features:**
- **Selective Transcoding**: Only processes AC4 channels, passes through standard AC3 channels untouched
- **HDHomeRun API Compatibility**: Complete transparent proxying with device ID management  
- **High Performance**: Direct `io.Copy` streaming with dependency injection architecture
- **AC4 Error Resilience**: Advanced FFmpeg error handling prevents stream crashes
- **Docker Ready**: Single-container deployment with embedded FFmpeg

## Quick Start

### Docker (Recommended)
```bash
# Pull from GitHub Container Registry
docker pull ghcr.io/attaebra/hdhr-proxy:latest

# Run the container
docker run --name hdhr-proxy -p 5003:80 -p 5004:5004 \
  -e HDHR_IP=192.168.50.200 \
  ghcr.io/attaebra/hdhr-proxy:latest
```

### VLC/Media Player Setup
Point your media player to `http://your-proxy-ip:5004` instead of your HDHomeRun's IP. The proxy automatically:
- Detects AC4 channels and transcodes them to EAC3
- Passes through regular AC3 channels without modification  
- Handles all HDHomeRun API calls transparently

## Architecture

### Clean & Focused Design
```
hdhr-proxy/
├── cmd/hdhr-proxy/          # Application entry point
├── internal/
│   ├── config/              # Streamlined configuration
│   ├── container/           # Dependency injection container
│   ├── interfaces/          # Clean DI contracts
│   ├── media/
│   │   ├── ffmpeg/          # AC4-resilient FFmpeg config
│   │   ├── stream/          # Direct io.Copy streaming
│   │   └── transcoder/      # FFmpeg process management
│   ├── proxy/               # HDHomeRun API proxying
│   └── utils/               # HTTP utilities
└── Dockerfile
```

### Key Design Principles
- **Dependency Injection**: Clean testable architecture via `internal/container/`
- **Direct Streaming**: `io.Copy` paths for minimal latency
- **Interface-Based**: All major components behind interfaces for testability
- **Resource Management**: Proper context cancellation and cleanup
- **Error Resilience**: AC4 decoding errors handled gracefully (up to 10 errors/stream)

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `HDHR_IP` | *required* | HDHomeRun device IP address |
| `LOG_LEVEL` | `info` | Logging level (debug, info, warn, error) |
| `FFMPEG_PATH` | `/usr/bin/ffmpeg` | FFmpeg executable path |

### Ports
- **5004**: Media streaming (HDHomeRun-compatible)
- **8080**: API/Discovery (HDHomeRun-compatible)

## Performance Features

### AC4 Error Resilience  
The proxy handles corrupted AC4 packets gracefully with **continuous streaming**:
```
AC4 decoding error (15 total, 3 consecutive): substream audio data overread
```
- **No hard error limit** - streams continue indefinitely
- **Smart error tracking** with sliding window approach  
- **Consecutive error monitoring** to detect stream quality issues
- Only logs warnings for high consecutive error rates

### Optimized Streaming
- **Zero-copy paths** where possible
- **Context-aware cancellation** for clean resource cleanup
- **Connection pooling** optimized for HDHomeRun usage patterns
- **Direct FFmpeg piping** without intermediate buffering

### HTTP Pipeline Optimization
- Consolidated request handling (eliminated ~180 lines of duplicate code)
- Shared connection setup and cleanup routines
- Optimized timeout settings for streaming vs API operations

## Development

### Requirements
- Go 1.24+
- FFmpeg (5.1+ recommended)
- Docker (for containerized deployment)

### Build & Test
```bash
# Build
go build ./cmd/hdhr-proxy

# Test
go test -v ./...

# Lint
golangci-lint run

# Docker
docker build -t hdhr-proxy .
```

### Architecture Notes
- **DI Container**: `internal/container/` manages all component lifecycles
- **Interfaces**: All dependencies injected via interfaces in `internal/interfaces/`
- **Direct Streaming**: No intermediate buffering - `HDHomeRun → FFmpeg → Client`
- **Process Management**: FFmpeg processes tracked per channel with proper cleanup

## Channels & Compatibility

The proxy automatically detects channel audio formats:
- **AC4 Channels**: Transcoded to EAC3 (384k, stereo)  
- **AC3 Channels**: Streamed directly (no transcoding)
- **All Other Formats**: Passed through unchanged

### Tested Media Players
- ✅ **VLC**: Full compatibility
- ✅ **Plex**: Complete HDHomeRun integration  
- ✅ **Emby**: Native HDHomeRun support
- ✅ **Jellyfin**: Works with HDHomeRun plugin

## Monitoring

### Health Check
```bash 
curl http://proxy-ip:8080/health
```

### Stream Status  
```bash
curl http://proxy-ip:5004/status
```
Shows active streams, FFmpeg processes, and system info.

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.