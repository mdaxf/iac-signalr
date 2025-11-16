# Use a lightweight Linux distribution as the base image
FROM golang:1.21-alpine AS builder

# Install ca-certificates for HTTPS support
RUN apk --no-cache add ca-certificates

# Set the working directory inside the container
WORKDIR /build

# Copy the Go module files
COPY go.mod go.sum ./

# Download the Go module dependencies
RUN go mod download

# Copy the entire application source
COPY . .

# Build the Go application with security flags
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -trimpath -o iac-signalrsrv-linux

# Final stage
FROM alpine:latest

# Install ca-certificates and create non-root user
RUN apk --no-cache add ca-certificates && \
    addgroup -g 1000 signalr && \
    adduser -D -u 1000 -G signalr signalr

WORKDIR /app

# Copy binary and required files with proper ownership
COPY --from=builder --chown=signalr:signalr /build/iac-signalrsrv-linux /app/iac-signalrsrv-linux
COPY --from=builder --chown=signalr:signalr /build/dockersignalrconfig.json /app/signalrconfig.json
COPY --from=builder --chown=signalr:signalr /build/public /app/public

# Set permissions on the application
RUN chmod +x iac-signalrsrv-linux

# Switch to non-root user
USER signalr

# Expose application port
EXPOSE 8222

# Add health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8222/ || exit 1

# Define an entry point to run the application
CMD ["./iac-signalrsrv-linux"]