# CLIENT-SIDE TIMEOUT FIX - 30 Second Disconnect Issue

## Root Cause Identified ✅

Your **server is correctly configured** with 60-second timeout, but your **CLIENT is using the default 30-second timeout**!

### Evidence from Server Logs

```
Server configured: Timeout: 60s  ← SERVER IS CORRECT
101 GET /iacmessagebus?id=... 30.0318227s  ← CLIENT DISCONNECTS AT 30s
```

The WebSocket connection (101 status = successful upgrade) lasts exactly **30.0318227 seconds**, which proves the **client** is timing out, not the server.

---

## The Problem

**SignalR TimeoutInterval Default = 30 seconds** (defined in `signalr/options.go:15`)

- ✅ **Server**: You configured `TimeoutInterval: 60s` in `signalrconfig.json`
- ❌ **Client**: Still using default `TimeoutInterval: 30s`

**Both server AND client must have matching (or compatible) timeout settings!**

---

## The Solution

Your Go client code **MUST** include these two critical settings:

```go
signalr.KeepAliveInterval(15*time.Second),  // Send ping every 15s
signalr.TimeoutInterval(60*time.Second),    // Disconnect after 60s of inactivity
```

---

## ✅ Required Client Configuration

### Option 1: Update Your Connect() Function

Add the timeout settings to your `signalr.NewClient()` call:

```go
func Connect() (*signalr.Client, error) {
    serverURL := "http://127.0.0.1:8222/iacmessagebus"

    ilog := logger.Log{
        ModuleName:     logger.SignalR,
        User:           "YourApp",
        ControllerName: "SignalR Client",
    }
    logAdapter := logger.NewSignalRLogAdapter(ilog)

    client, err := signalr.NewClient(
        context.Background(),
        signalr.WithReceiver(&MessageReceiver{}),
        signalr.WithConnector(func() (signalr.Connection, error) {
            ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
            defer cancel()

            return signalr.NewHTTPConnection(ctx, serverURL,
                signalr.WithTransports(signalr.TransportWebSockets), // WebSocket only!
            )
        }),
        signalr.Logger(logAdapter, false),

        // ⭐ CRITICAL: Add these two lines! ⭐
        signalr.KeepAliveInterval(15*time.Second),
        signalr.TimeoutInterval(60*time.Second),
    )

    if err != nil {
        return nil, fmt.Errorf("failed to create SignalR client: %w", err)
    }

    return client, nil
}
```

### Option 2: Minimal Fix (Just Add These Lines)

If you already have a working client, just add these two lines to your `signalr.NewClient()` options:

```go
client, err := signalr.NewClient(
    context.Background(),
    // ... your existing options ...
    signalr.KeepAliveInterval(15*time.Second),  // ← ADD THIS
    signalr.TimeoutInterval(60*time.Second),    // ← ADD THIS
)
```

---

## Complete Working Example

See `/home/user/iac-signalr/clientsample/client_modern.go` lines 181-197 for a complete working example.

Key sections:

```go
client, err := signalr.NewClient(
    context.Background(),
    signalr.WithReceiver(&MessageReceiver{}),
    signalr.WithConnector(func() (signalr.Connection, error) {
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()

        return signalr.NewHTTPConnection(ctx, serverURL,
            signalr.WithTransports(signalr.TransportWebSockets),
        )
    }),
    signalr.Logger(logAdapter, false),
    signalr.KeepAliveInterval(15*time.Second),   // ← MUST MATCH SERVER
    signalr.TimeoutInterval(60*time.Second),     // ← MUST MATCH SERVER
)
```

---

## Configuration Alignment

| Setting | Server (signalrconfig.json) | Client (Go code) | Status |
|---------|----------------------------|------------------|---------|
| **KeepAliveInterval** | 15 seconds | **15*time.Second** | ✅ Match |
| **TimeoutInterval** | 60 seconds | **60*time.Second** | ✅ Match |
| **Transport** | WebSocket-only | **TransportWebSockets** | ✅ Match |
| **InsecureSkipVerify** | true | N/A (server only) | ✅ OK |

**Rule**: `TimeoutInterval` should be **at least 2x KeepAliveInterval**
- KeepAlive: 15s → Timeout: 30s minimum (we use 60s for extra safety)

---

## Testing After Fix

After adding the timeout settings to your client:

1. **Start your server** (should already be running with correct config)
2. **Start your client** with the updated code
3. **Monitor the logs** - you should see:

```
✅ Expected Result:
2025/11/15 21:xx:xx [I] Client xxx connected and joining group IAC_Internal_MessageBus
... (no disconnect for at least 60+ seconds) ...
```

❌ **Before (30s disconnect):**
```
21:24:07 connected
21:24:37 disconnected  ← exactly 30s later
```

✅ **After (stable connection):**
```
21:24:07 connected
... client stays connected indefinitely (as long as it's active)
```

---

## Why Both Server AND Client Need Matching Timeouts

**SignalR is a bidirectional protocol** - both parties monitor the connection:

1. **Server monitors client**: "If I don't hear from client in 60s, disconnect"
2. **Client monitors server**: "If I don't hear from server in 60s, disconnect"

If client timeout (30s) < server keepalive (15s):
- ❌ Client disconnects before server even notices!

If client timeout (60s) = server timeout (60s):
- ✅ Both parties agree on connection health

---

## Additional Fixes for 301 Redirects

The `301 GET //iacmessagebus` redirects are harmless but can be eliminated by ensuring:

1. **Client URL has single slash**: `http://127.0.0.1:8222/iacmessagebus` (not `//iacmessagebus`)
2. **No trailing slashes**: Don't use `http://127.0.0.1:8222/iacmessagebus/`

---

## Summary

**Problem**: Client using default 30s timeout while server uses 60s timeout
**Solution**: Add `signalr.TimeoutInterval(60*time.Second)` to client configuration
**Result**: Client will stay connected as long as server, no more 30-second disconnects!

---

## Quick Reference

```go
// Import required packages
import (
    "context"
    "time"
    "github.com/mdaxf/iac-signalr/signalr"
)

// When creating client, add these options:
client, err := signalr.NewClient(
    context.Background(),
    // ... other options ...
    signalr.KeepAliveInterval(15*time.Second),
    signalr.TimeoutInterval(60*time.Second),
)
```

**That's it!** 🎉
