#!/bin/bash

# Make sure the HDHR_IP environment variable is set
if [ -z "$HDHR_IP" ]; then
  echo "HDHR_IP environment variable not set"
  exit 1
fi

# Download the correct Emby version for this architecture
echo "Detected architecture: $(uname -m)"
if [ "$LINK" ]; then
  echo "Overriding Emby release"
  LINK=$LINK
elif [ "$(uname -m)" == "x86_64" ]; then
  LINK="https://github.com/MediaBrowser/Emby.Releases/releases/download/4.8.10.0/emby-server-deb_4.8.10.0_amd64.deb"
elif [ "$(uname -m)" == "aarch64" ]; then
  LINK="https://github.com/MediaBrowser/Emby.Releases/releases/download/4.8.10.0/emby-server-deb_4.8.10.0_arm64.deb"
else
  echo "Unknown architecture. Set the LINK environment variable to a URL of a .deb file from https://github.com/MediaBrowser/Emby.Releases/releases"
  exit 1
fi

echo "Downloading Emby from $LINK"
curl -L -o emby.deb $LINK
ar x emby.deb data.tar.xz
tar xf data.tar.xz

# Put the ffmpeg binaries in the right place, ignoring missing files
mv opt/emby-server/bin/ffmpeg /usr/bin/ffmpeg
mv opt/emby-server/lib/libav*.so.* /usr/lib/ || true
mv opt/emby-server/lib/libpostproc.so.* /usr/lib/ || true
mv opt/emby-server/lib/libsw* /usr/lib/ || true
mv opt/emby-server/extra/lib/libva*.so.* /usr/lib/ || true
mv opt/emby-server/extra/lib/libdrm.so.* /usr/lib/ || true
mv opt/emby-server/extra/lib/libmfx.so.* /usr/lib/ || true
mv opt/emby-server/extra/lib/libOpenCL.so.* /usr/lib/ || true

# Start the server
echo "Starting hdhr-proxy..."
/app/hdhr-proxy -hdhr-ip $HDHR_IP -log-level $LOG_LEVEL 