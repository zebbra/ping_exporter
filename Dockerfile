# Build stage
FROM golang:1.23-alpine AS builder

# Install ca-certificates for SSL/TLS
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ping_exporter .

# Final stage
FROM alpine:latest

# Install ca-certificates for SSL/TLS
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/ping_exporter .

# Expose port
EXPOSE 9115

# Ping requires NET_RAW capability, so we run as root
# Container should be run with --cap-add=NET_RAW
USER root

# Run the exporter
ENTRYPOINT [ "/app/ping_exporter" ]
