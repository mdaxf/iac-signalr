
# Use a lightweight Linux distribution as the base image
FROM golang:1.21-alpine AS builder
# Set the working directory inside the container
WORKDIR /build

# Copy the Go module files
COPY go.mod go.sum ./

# Download the Go module dependencies
RUN go mod download

# Copy the entire application source
COPY . .

# Build the Go application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o iac-signalrsrv-linux

# Final stage
FROM alpine:latest

WORKDIR /app

COPY --from=builder /build/iac-signalrsrv-linux /app/iac-signalrsrv-linux
COPY --from=builder /build/dockersignalrconfig.json /app/signalrconfig.json
COPY --from=builder /build/public    /app/public

# Set permissions on the application (if needed)
RUN chmod +x iac-signalrsrv-linux


# Expose additional ports
EXPOSE 8222
# Define an entry point to run the application

CMD ["./iac-signalrsrv-linux"]