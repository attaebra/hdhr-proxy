# Build stage
FROM golang:1.24 AS builder

WORKDIR /app

# Copy go.mod first to leverage Docker cache
COPY go.mod ./
# Conditionally copy go.sum if it exists
COPY go.sum* ./
# Initialize go.sum if it doesn't exist
RUN touch go.sum && go mod download

# Copy source code
COPY . .

# Build the Go application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o hdhr-proxy ./cmd/hdhr-proxy

# Final stage
FROM debian:bullseye-slim

# Install dependencies needed for extracting ffmpeg from Emby
RUN apt-get update && apt-get install -y \
    curl \
    binutils \
    xz-utils \
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

CMD ["/bin/bash", "/app/run.sh"] 