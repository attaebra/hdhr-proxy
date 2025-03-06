# HDHR Proxy

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
hdhr-proxy/
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
docker run -p 5003:80 -p 5004:5004 -e HDHR_IP=192.168.1.101 ghcr.io/attaebra/hdhr-proxy:latest
```

Replace `192.168.1.101` with the IP address of your HDHomeRun device.

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
3. Enter the container's IP and port (e.g., `192.168.1.100:5003`)
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
   git clone https://github.com/attaebra/hdhr-proxy.git
   cd hdhr-proxy
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

Run tests with coverage report:
```bash
go test -cover ./...
```

Generate a detailed HTML coverage report:
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

#### Testing Specific Packages

Test only the proxy package:
```bash
go test ./internal/proxy
```

Test only the media package:
```bash
go test ./internal/media
```

#### Test Environment Variables

Some tests can be customized with environment variables:

- `LOG_LEVEL`: Set logging level (debug, info, warn, error) during tests
- `CI`: Set to "true" to skip tests that require external resources

#### Manual Testing

For manual testing with the HDHomeRun:

1. Start the application with your HDHomeRun IP:
   ```bash
   LOG_LEVEL=debug ./hdhr-proxy -hdhr-ip 192.168.1.101
   ```

2. Test the API endpoints:
   ```bash
   curl http://localhost:80/discover.json
   curl http://localhost:80/lineup.json
   ```

3. Test the media streaming:
   ```bash
   # Using VLC:
   vlc http://localhost:5004/auto/v5.1
   
   # Using ffplay:
   ffplay http://localhost:5004/auto/v5.1
   ```

4. Check the status endpoint:
   ```bash
   curl http://localhost:5004/status
   ```

### Linting

The project uses golangci-lint for code quality and style checks.

#### Installing the Linter

```bash
# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

#### Running the Linter

Run the linter on the entire codebase:
```bash
golangci-lint run
```

Run the linter with specific checks:
```bash
golangci-lint run --enable=govet,errcheck,staticcheck,unused,gosimple
```

Generate a report in various formats:
```bash
golangci-lint run --out-format=json > lint-report.json
```

Fix automatically fixable issues:
```bash
golangci-lint run --fix
```

#### Integrating with CI

The project's CI workflow automatically runs linting checks on every pull request.
You can view the linting configuration in `.golangci.yml` at the root of the repository.

### Docker Build

```bash
docker build -t hdhr-proxy .
```

## Troubleshooting

### Common Issues

1. **Connection Timeout**: Ensure your HDHomeRun IP is correct and accessible
2. **Missing Channels**: Verify that your HDHomeRun can receive the channel
3. **Transcoding Errors**: Check that FFmpeg is properly extracted (watch container logs)

### Logs

Container logs will show download progress, FFmpeg extraction, and any errors:

```bash
docker logs hdhr-proxy
```

Set the `LOG_LEVEL` environment variable to `debug` for more detailed logs:

```bash
docker run -e HDHR_IP=192.168.1.101 -e LOG_LEVEL=debug -p 5003:80 -p 5004:5004 ghcr.io/attaebra/hdhr-proxy:latest
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Acknowledgments & Improvements

This project was inspired by [whichken/hdhr-ac4](https://github.com/whichken/hdhr-ac4), a Node.js implementation that proxies HDHomeRun ATSC 3.0 tuners to transcode AC4 audio. While sharing a similar goal, this Go implementation offers several enhancements:

### Key Improvements

- **Go Implementation**: Written in Go for better performance, lower resource usage, and easier deployment
- **Selective Transcoding**: Only transcodes channels with AC4 audio, passing through other channels directly without transcoding
- **Smart Connection Management**: Properly detects client disconnections and immediately releases tuner resources
- **Configurable Logging Levels**: Supports debug, info, warn, and error log levels configurable via environment variables

Like the original project, this is released under the Apache 2.0 license.
