# Build stage
FROM golang:1.24 AS builder

WORKDIR /app

# Copy the Go modules manifests
COPY go.mod go.sum ./
# Download dependencies (if go.sum exists)
RUN go mod download || true

# Copy source code
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY pkg/ ./pkg/

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o hdhr-proxy ./cmd/hdhr-proxy

# Final stage
FROM debian:bullseye-slim

# Install dependencies needed for extracting ffmpeg from Emby
RUN apt-get update && apt-get install -y \
    curl \
    binutils \
    xz-utils \
    libfontconfig1 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy only the necessary files from the builder stage
COPY --from=builder /app/hdhr-proxy /app/
COPY run.sh /app/

RUN chmod +x /app/run.sh

# Expose ports
EXPOSE 80
EXPOSE 5004

# Set environment variable defaults
ENV HDHR_IP=""
ENV LINK=""
ENV LOG_LEVEL="info"

CMD ["/bin/bash", "/app/run.sh"]