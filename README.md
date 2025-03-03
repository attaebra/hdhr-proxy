# hdhr-ac4-go

A Go implementation of a proxy for HDHomeRun ATSC 3.0 tuners that converts AC4 audio to AC3 for compatibility with more media players.

## What does this do?

This is a Docker container that:

1. Proxies requests from media players to an ATSC 3.0 compatible HDHomeRun tuner
2. Uses ffmpeg (extracted from Emby media server) to transcode AC4 audio to AC3 audio on-the-fly
3. Makes appropriate substitutions in API responses for compatibility

This container allows media players like Plex and VLC to work with ATSC 3.0 content that uses AC4 audio.

## Features

- High-performance proxy written in Go
- Automatic device ID detection and reversal (so the emulated tuner can coexist with the real one)
- Support for custom ports
- Transparent proxying of HDHomeRun API
- Efficient media streaming with minimal overhead
- Comprehensive test suite

## How to use this docker container

```bash
docker run -p 5003:80 -p 5004:5004 -e HDHR_IP=192.168.0.123 hdhr-ac4-go
```

On startup, the container will download an Emby release and extract ffmpeg from it. It will detect amd64 or arm64 architectures and download the appropriate release. You can override the release it uses by setting the `LINK` environment variable to a URL of a .deb file from [Emby Releases](https://github.com/MediaBrowser/Emby.Releases/releases).

> You can use any host port you want for port 80, but port 5004 can't be changed.

Now go to Plex and add a new tuner. Enter the container IP and your chosen port (ex. 192.168.0.234:5003) as the tuner address. Plex should see and add it without issue.

## Development

### Prerequisites

- Go 1.24 or later
- Docker (for building the container)

### Building Locally

1. Clone the repository
2. Run the unit tests: `go test ./...`
3. Build the application: `go build ./cmd/hdhr-proxy`

### Testing with VLC

You can test the proxy with VLC by:

1. Start the proxy with `-hdhr-ip` flag pointing to your HDHomeRun tuner
2. Open VLC and go to "Open Network Stream"
3. Enter the URL: `http://localhost:5004/auto/v5.1` (replacing v5.1 with your channel number)

### Docker Build

```bash
docker build -t hdhr-ac4-go .
```

## License

Apache License 2.0 