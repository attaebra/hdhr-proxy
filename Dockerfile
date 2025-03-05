# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy necessary files
COPY go.mod go.sum ./
RUN go mod download || true

# Copy source code
COPY . .

# Build the application
RUN go build -o hdhr-proxy ./cmd/hdhr-proxy

# Final stage
FROM debian:bullseye-slim

WORKDIR /app

# Install dependencies needed for extracting ffmpeg from Emby
RUN apt-get update && apt-get install -y \
    curl \
    binutils \
    xz-utils \
    ca-certificates \
    libfontconfig1 \
    && rm -rf /var/lib/apt/lists/*

# Copy binary from build stage
COPY --from=builder /app/hdhr-proxy /app/hdhr-proxy
COPY run.sh /app/

RUN chmod +x /app/hdhr-proxy /app/run.sh

# Create necessary directories
RUN mkdir -p /usr/lib /usr/bin

# Expose ports
EXPOSE 80
EXPOSE 5004

# Set environment variable defaults
ENV HDHR_IP=""
ENV LINK=""
ENV LOG_LEVEL="info"

CMD ["/bin/bash", "/app/run.sh"]