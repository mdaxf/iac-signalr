# IAC SignalR Server

A high-performance SignalR server implementation in Go for real-time communication and messaging.

## Features

- **Real-time Messaging**: WebSocket and Server-Sent Events (SSE) support
- **Group Management**: Subscribe and broadcast messages to client groups
- **Structured Logging**: Comprehensive logging with multiple adapters (console, file, MongoDB, etc.)
- **Health Monitoring**: Built-in health check endpoint and heartbeat mechanism
- **CORS Support**: Configurable Cross-Origin Resource Sharing
- **Secure Authentication**: API key-based authentication with constant-time comparison
- **Docker Support**: Containerized deployment with multi-stage builds

## Quick Start

### Prerequisites

- Go 1.21.4 or higher
- Docker (optional, for containerized deployment)

### Installation

```bash
# Clone the repository
git clone https://github.com/mdaxf/iac-signalr.git
cd iac-signalr

# Install dependencies
go mod download

# Build the application
go build -o iac-signalr
```

### Configuration

Create a `signalrconfig.json` file (or copy from `signalrconfig.json.example`):

```json
{
    "address": "0.0.0.0:8222",
    "clients": "http://127.0.0.1:8080,https://127.0.0.1:8080",
    "insecureSkipVerify": false,
    "keepAliveInterval": 15,
    "timeoutInterval": 60,
    "auth": {
        "enabled": true,
        "token": "your-secure-token-here"
    },
    "appserver": {
        "url": "http://127.0.0.1:8080",
        "apikey": "your-secret-api-key"
    },
    "log": {
        "adapter": "console",
        "level": "info"
    }
}
```

**Important**: Never commit your actual configuration files with sensitive data. Use environment variables for production deployments.

### Environment Variables

The server supports the following environment variables (which override configuration file values):

- `SIGNALR_CONFIG`: Path to configuration file (default: `signalrconfig.json`)
- `SIGNALR_ADDRESS`: Server listen address (e.g., `0.0.0.0:8222`)
- `SIGNALR_CLIENTS`: Allowed client origins (comma-separated)
- `SIGNALR_API_KEY`: API key for health endpoint authentication (recommended over config file)
- `SIGNALR_AUTH_ENABLED`: Set to `true` to enable SignalR connection authorization
- `SIGNALR_AUTH_TOKEN`: Token for SignalR connection authorization (recommended over config file)
- `SIGNALR_INSECURE_SKIP_VERIFY`: Set to `true` to disable origin verification (not recommended for production)

### Running the Server

```bash
# Using the binary
./iac-signalr

# Or with Go
go run .

# With environment variables
SIGNALR_API_KEY=your-secret-key SIGNALR_ADDRESS=0.0.0.0:8222 ./iac-signalr
```

### Docker Deployment

```bash
# Build the Docker image
docker build -t iac-signalr:latest .

# Run the container
docker run -d \
  -p 8222:8222 \
  -e SIGNALR_API_KEY=your-secret-key \
  -e SIGNALR_CLIENTS=http://your-frontend.com \
  --name iac-signalr \
  iac-signalr:latest
```

## API Documentation

### Health Check Endpoint

**Endpoint**: `GET /health`

**Headers**:
- `Authorization`: `apikey <your-api-key>`

**Response**:
```json
{
    "Node": {
        "Name": "iac-signalr",
        "AppID": "uuid",
        "Host": "hostname",
        "IPAddress": "192.168.1.1",
        "OS": "linux",
        "Version": "1.0.0",
        "Status": "Running"
    },
    "Result": {
        "status": "healthy"
    },
    "timestamp": "2025-01-15T12:00:00Z"
}
```

### SignalR Hub Methods

The IAC Message Bus hub supports the following methods:

#### Subscribe
Subscribe to a topic for receiving messages.

```javascript
connection.invoke("Subscribe", topic, connectionId);
```

#### Send
Send a message to all subscribers of a topic.

```javascript
connection.invoke("Send", topic, message, connectionId);
```

#### SendToBackEnd
Send a message specifically to backend listeners.

```javascript
connection.invoke("SendToBackEnd", topic, message, connectionId);
```

#### Broadcast
Broadcast a message to all connected clients.

```javascript
connection.invoke("Broadcast", message);
```

## Logging

The server supports multiple logging adapters:

- **Console**: Standard output logging
- **File**: File-based logging with rotation
- **MultiFile**: Separate log files per level
- **DocumentDB**: MongoDB logging
- **SMTP**: Email notifications for critical errors

Configure logging in `signalrconfig.json`:

```json
{
    "log": {
        "adapter": "console",
        "level": "info"
    }
}
```

Log levels (from most to least severe):
- `emergency`
- `alert`
- `critical`
- `error`
- `warning`
- `notice`
- `info`
- `debug`

## Authorization

### SignalR Connection Authorization

The server supports token-based authorization for SignalR connections. When enabled, clients must provide a valid token to establish connections and exchange data.

#### Configuration

Enable authorization in `signalrconfig.json`:

```json
{
    "auth": {
        "enabled": true,
        "token": "your-secure-token-here"
    }
}
```

Or use environment variables (recommended):

```bash
export SIGNALR_AUTH_ENABLED=true
export SIGNALR_AUTH_TOKEN=your-secure-token-here
```

#### Client Authentication

Clients can provide the token in two ways:

**1. Authorization Header (Recommended)**:
```javascript
const connection = new signalR.HubConnectionBuilder()
    .withUrl("/iacmessagebus", {
        accessTokenFactory: () => "your-secure-token-here"
    })
    .build();
```

**2. Query Parameter**:
```javascript
const connection = new signalR.HubConnectionBuilder()
    .withUrl("/iacmessagebus?access_token=your-secure-token-here")
    .build();
```

#### Behavior

- **When enabled**: All connection attempts (negotiate, WebSocket, SSE) require a valid token
- **Invalid/missing token**: Connection is rejected with HTTP 401 Unauthorized
- **When disabled**: No token validation is performed (not recommended for production)

## Security Best Practices

1. **Always use environment variables** for sensitive data (API keys, tokens, connection strings)
2. **Enable SignalR authorization** in production environments
3. **Enable HTTPS** in production environments
4. **Set `insecureSkipVerify: false`** in production
5. **Restrict CORS origins** to only trusted domains
6. **Use strong tokens/API keys** (at least 32 characters, random)
7. **Rotate tokens regularly** and implement token expiration if needed
8. **Keep dependencies updated** regularly

## Development

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific package tests
go test ./signalr/...
```

### Building for Different Platforms

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o iac-signalr-linux

# Windows
GOOS=windows GOARCH=amd64 go build -o iac-signalr.exe

# macOS
GOOS=darwin GOARCH=amd64 go build -o iac-signalr-darwin
```

## Architecture

```
iac-signalr/
├── signalr/          # Core SignalR protocol implementation
├── middleware/       # HTTP middleware (CORS, logging)
├── logger/           # Logging infrastructure
├── router/           # HTTP router implementations
├── public/           # Static file serving
├── client/           # Client implementations
├── chatsample/       # Example chat application
└── server.go         # Main server entry point
```

## Troubleshooting

### Connection Issues

1. Check firewall rules allow traffic on the configured port
2. Verify CORS settings match your client origin
3. Ensure WebSocket support is enabled in your proxy/load balancer

### Authentication Failures

**Health Endpoint (/health)**:
1. Verify API key is correctly set in environment or config
2. Check Authorization header format: `apikey <your-key>`
3. Ensure no whitespace or special characters in the API key

**SignalR Connections**:
1. Check if authorization is enabled (`SIGNALR_AUTH_ENABLED=true`)
2. Verify token is correctly provided via Authorization header or query parameter
3. Ensure token matches the configured value exactly (case-sensitive)
4. Check browser console for 401 Unauthorized errors
5. Verify client is using `accessTokenFactory` or `access_token` query parameter

### Performance Issues

1. Monitor log level - debug logging can impact performance
2. Check connection limits and system resources
3. Consider using load balancing for high traffic

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Commit your changes with clear messages
4. Add tests for new functionality
5. Submit a pull request

## License

Licensed under the Apache License, Version 2.0. See [LICENSE.md](LICENSE.md) for details.

## Support

For issues and questions:
- Open an issue on GitHub
- Check existing documentation and examples
- Review the chat sample application for implementation guidance

## Changelog

### Version 1.1.0 (2025-01-19)

#### New Features
- **SignalR Connection Authorization**: Added token-based authorization for SignalR connections
  - Configurable via `auth.enabled` and `auth.token` in config or environment variables
  - Supports both Authorization header (Bearer token) and query parameter authentication
  - Validates tokens on negotiate, WebSocket, and SSE endpoints
  - Uses constant-time comparison to prevent timing attacks

#### Security Improvements
- Added `SIGNALR_AUTH_ENABLED` and `SIGNALR_AUTH_TOKEN` environment variables
- Unauthorized connection attempts are rejected with HTTP 401

#### Documentation
- Added comprehensive authorization documentation to README
- Updated configuration examples with auth settings
- Added troubleshooting guide for authentication issues

### Version 1.0.0 (2025-01-15)

#### Security Improvements
- Replaced hardcoded API keys with environment variable support
- Implemented constant-time string comparison for authentication
- Made `InsecureSkipVerify` configurable (defaults to false)

#### Logging Enhancements
- Replaced verbose header logging with structured logging
- Implemented SignalRLogAdapter for go-kit compatibility
- Removed excessive debug output from production code

#### Code Quality
- Replaced deprecated `ioutil` with `io` and `os` packages
- Fixed potential nil pointer dereference in GetHostandIPAddress
- Improved error handling in main() and server initialization
- Added comprehensive .gitignore

#### Documentation
- Added complete README with examples
- Documented all configuration options
- Included security best practices
- Added troubleshooting guide
