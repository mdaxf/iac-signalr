# Connection Stability Fixes

## Problem
Clients were experiencing automatic connect/disconnect loops with the following symptoms:
- Client connects successfully
- Immediately disconnects
- Reconnects automatically
- Pattern repeats continuously
- 301 redirect on `/iacmessagebus` path (double slash issue)

## Root Causes Identified

### 1. Missing InsecureSkipVerify Configuration
The server was defaulting to `insecureSkipVerify: false`, enabling origin verification. If client origins didn't match the allowed patterns exactly, connections were rejected.

### 2. Improper Timeout Configuration
- **KeepAliveInterval**: 10 seconds
- **TimeoutInterval**: Not set (defaulted to 30 seconds)
- **HandshakeTimeout**: Not set (defaulted to 15 seconds)

While the defaults were functional, they weren't explicitly configured, leading to potential timing issues.

### 3. Inadequate Connection Logging
Connection and disconnection events weren't logged at the appropriate level, making debugging difficult.

## Solutions Implemented

### 1. Updated Server Configuration

**File: `server.go:71-90`**

```go
// Configure server with proper timeout settings
// TimeoutInterval should be at least 2x KeepAliveInterval
server, err := signalr.NewServer(context.TODO(), signalr.SimpleHubFactory(hub),
    signalr.Logger(logAdapter, false),
    signalr.KeepAliveInterval(15*time.Second),
    signalr.TimeoutInterval(30*time.Second),
    signalr.HandshakeTimeout(15*time.Second),
    signalr.AllowOriginPatterns([]string{clients}),
    signalr.InsecureSkipVerify(insecureSkipVerify))
```

**Changes:**
- KeepAliveInterval: 15 seconds (increased from 10)
- TimeoutInterval: 30 seconds (explicitly set, was default)
- HandshakeTimeout: 15 seconds (explicitly set, was default)
- Configuration is now logged on startup

### 2. Enhanced Connection Logging

**File: `signalr.go:64-72`**

```go
func (c *IACMessageBus) OnConnected(connectionID string) {
    c.ilog.Info(fmt.Sprintf("Client %s connected and joining group %s", connectionID, groupname))
    c.Groups().AddToGroup(groupname, connectionID)
}

func (c *IACMessageBus) OnDisconnected(connectionID string) {
    c.ilog.Info(fmt.Sprintf("Client %s disconnected from group %s", connectionID, groupname))
    c.Groups().RemoveFromGroup(groupname, connectionID)
}
```

**Changes:**
- Connection events now logged at INFO level (was DEBUG)
- Clearer log messages for troubleshooting
- Reduced log duplication

### 3. Configuration File Examples

**File: `signalrconfig.json.example`**

Created an example configuration with recommended settings:

```json
{
    "address": "0.0.0.0:8222",
    "clients": "http://127.0.0.1:8080,https://127.0.0.1:8080,http://localhost:8080,https://localhost:8080",
    "insecureSkipVerify": true,
    "appserver": {
        "url": "http://127.0.0.1:8080",
        "apikey": "your-secret-api-key-here"
    },
    "log": {
        "adapter": "console",
        "level": "info"
    }
}
```

## How to Fix Your Installation

### Option 1: Add insecureSkipVerify to Config (Quick Fix)

Add this field to your `signalrconfig.json`:

```json
{
    "address": "0.0.0.0:8222",
    "clients": "http://127.0.0.1:8080,https://127.0.0.1:8080",
    "insecureSkipVerify": true,   <-- ADD THIS LINE
    ...
}
```

### Option 2: Use Environment Variable (Recommended for Development)

```bash
export SIGNALR_INSECURE_SKIP_VERIFY=true
./iac-signalr
```

### Option 3: Configure Proper Origin Patterns (Production)

If you know the exact origins your clients will use, add them all to the clients list:

```json
{
    "clients": "http://127.0.0.1:8080,https://127.0.0.1:8080,http://localhost:8080,https://localhost:8080,http://your-domain.com,https://your-domain.com"
}
```

Then set `insecureSkipVerify: false` for production security.

## Testing the Fix

1. **Update your configuration**:
   ```bash
   # Add to signalrconfig.json
   "insecureSkipVerify": true
   ```

2. **Restart the server**:
   ```bash
   ./iac-signalr
   ```

3. **Monitor the logs**:
   ```
   2025/11/15 18:26:47 [I] SignalR server configured - KeepAlive: 15s, Timeout: 30s, InsecureSkipVerify: true
   2025/11/15 18:26:48 [I] Client K7tJub2mTGS_96mTW-z6sw== connected and joining group IAC_Internal_MessageBus
   ```

4. **Expected behavior**:
   - Client connects
   - Stays connected
   - No automatic disconnections
   - No 301 redirects

## Understanding the Settings

### KeepAliveInterval (15 seconds)
The server sends a ping to the client every 15 seconds if no other messages have been sent. This keeps the connection alive and detects disconnections.

### TimeoutInterval (30 seconds)
If the server doesn't receive any message from the client within 30 seconds, it considers the client disconnected. Should be at least 2x KeepAliveInterval.

### HandshakeTimeout (15 seconds)
The client must complete the SignalR handshake within 15 seconds of connecting.

### InsecureSkipVerify
- **true**: Accepts connections from any origin (use for development/testing)
- **false**: Only accepts connections from origins listed in "clients" (use for production with proper origin list)

## Troubleshooting

### Still experiencing disconnections?

1. **Check client origin**:
   ```bash
   # In browser console or network tab, verify the Origin header
   # Make sure it matches one of the patterns in "clients"
   ```

2. **Check for client-side errors**:
   - Open browser developer tools
   - Look for WebSocket errors or failed connection attempts
   - Check console for SignalR client errors

3. **Enable debug logging**:
   ```json
   {
       "log": {
           "level": "debug"
       }
   }
   ```

4. **Verify network connectivity**:
   - Check firewall rules allow WebSocket connections
   - Verify no proxy is interfering with WebSocket upgrade
   - Ensure the server port (8222) is accessible

### 301 Redirect Issue

If you still see `301 GET //iacmessagebus`, this indicates a path construction problem on the client side. Ensure your client is connecting to:

**Correct**: `http://localhost:8222/iacmessagebus`
**Incorrect**: `http://localhost:8222//iacmessagebus` (double slash)

Check your client connection code for proper URL construction.

## Performance Considerations

With the new settings:
- **Bandwidth**: Minimal increase due to keep-alive pings every 15 seconds
- **Connection stability**: Significantly improved
- **Timeout detection**: 30 seconds (reasonable for most use cases)

For high-latency connections, you may want to increase these values:
```go
signalr.KeepAliveInterval(30*time.Second),
signalr.TimeoutInterval(60*time.Second),
```

## Security Notes

**Development**:
- `insecureSkipVerify: true` is acceptable

**Production**:
- Set `insecureSkipVerify: false`
- Explicitly list all allowed origins in "clients"
- Use HTTPS for all client connections
- Consider using `SIGNALR_API_KEY` environment variable for API key

## Related Documentation

- [SignalR Configuration](./README.md#configuration)
- [Security Best Practices](./README.md#security-best-practices)
- [Troubleshooting Guide](./README.md#troubleshooting)
