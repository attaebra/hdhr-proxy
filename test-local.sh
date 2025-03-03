#!/bin/bash

# Exit on error
set -e

# Function to check if a command exists
command_exists() {
  command -v "$1" >/dev/null 2>&1
}

# Check for required dependencies
if ! command_exists go; then
  echo "Error: Go is not installed or not in PATH"
  exit 1
fi

if ! command_exists curl; then
  echo "Error: curl is not installed or not in PATH"
  exit 1
fi

# Check for extraction tools
if ! command_exists ar; then
  echo "Error: ar is not installed or not in PATH. Please install the 'binutils' package."
  exit 1
fi

if ! command_exists tar; then
  echo "Error: tar is not installed or not in PATH"
  exit 1
fi

# Check for HDHomeRun IP
if [ -z "$1" ]; then
  echo "Usage: $0 <hdhr-ip-address>"
  echo "Example: $0 192.168.1.100"
  exit 1
fi

HDHR_IP="$1"
APP_PORT=8080
MEDIA_PORT=5004
TEMP_DIR=$(mktemp -d)
FFMPEG_PATH="$TEMP_DIR/ffmpeg"

echo "Setting up Emby FFmpeg in temporary directory: $TEMP_DIR"

# Download the correct Emby version for this architecture
echo "Detected architecture: $(uname -m)"
if [ "$(uname -m)" == "x86_64" ] || [ "$(uname -m)" == "amd64" ]; then
  LINK="https://github.com/MediaBrowser/Emby.Releases/releases/download/4.8.10.0/emby-server-deb_4.8.10.0_amd64.deb"
elif [ "$(uname -m)" == "arm64" ] || [ "$(uname -m)" == "aarch64" ]; then
  LINK="https://github.com/MediaBrowser/Emby.Releases/releases/download/4.8.10.0/emby-server-deb_4.8.10.0_arm64.deb"
else
  echo "Unknown architecture: $(uname -m)"
  echo "This script only supports x86_64/amd64 or arm64/aarch64 architectures"
  exit 1
fi

echo "Downloading Emby from $LINK"
cd "$TEMP_DIR"
curl -L -o emby.deb "$LINK"
ar x emby.deb data.tar.xz
tar xf data.tar.xz

if [ ! -f "opt/emby-server/bin/ffmpeg" ]; then
  echo "Error: ffmpeg binary not found in the extracted package"
  exit 1
fi

cp opt/emby-server/bin/ffmpeg "$FFMPEG_PATH"
chmod +x "$FFMPEG_PATH"

echo "FFmpeg extracted successfully: $FFMPEG_PATH"

# Return to the original directory
cd - >/dev/null

# Build the application
echo "Building hdhr-proxy..."
go build -o hdhr-proxy ./cmd/hdhr-proxy

echo "Starting hdhr-proxy with HDHomeRun IP: $HDHR_IP"
echo "App server on port $APP_PORT, Media server on port $MEDIA_PORT"
echo "Using FFmpeg from: $FFMPEG_PATH"
echo ""
echo "Test in VLC by opening a network stream to: http://localhost:$MEDIA_PORT/auto/v5.1"
echo "(Replace v5.1 with your actual channel number)"
echo ""
echo "Press Ctrl+C to stop the server"
echo ""

# Start the proxy with the provided HDHomeRun IP
./hdhr-proxy -hdhr-ip "$HDHR_IP" -app-port "$APP_PORT" -media-port "$MEDIA_PORT" -ffmpeg "$FFMPEG_PATH"

# Cleanup
echo "Cleaning up temporary files..."
rm -rf "$TEMP_DIR" 