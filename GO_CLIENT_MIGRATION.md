# Go Client Migration Guide - Fix Auto-Disconnect

## Problem Overview

Your current Go clients (`client/client.go` and `clientsample/client.go`) use **SignalR 1.x protocol**, which is incompatible with your **SignalR Core server**. This causes:

- ❌ 30-second automatic disconnections
- ❌ 301 redirects with double slashes
- ❌ Wrong negotiation URLs
- ❌ Protocol version mismatch
- ❌ No proper keep-alive handling

## Root Cause

### Current Client Issues

**File: `clientsample/client.go`**

```go
// ❌ WRONG: Old SignalR 1.x negotiation
func negotiate(scheme, address string, hub string) (negotiationResponse, error) {
    urlpath := fmt.Sprintf("%s/%s", hub, "negotiate")  // Creates "iacmessagebus/negotiate"
    var negotiationUrl = url.URL{Scheme: scheme, Host: address, Path: urlpath}
    // ...
}

// ❌ WRONG: URL construction creates double slashes
urlpath := fmt.Sprintf("/%s", hub)  // Creates "/iacmessagebus"
var connectionUrl = url.URL{Scheme: "ws", Host: address, Path: urlpath}
// When combined with base URL, creates: //iacmessagebus
```

### What Happens

1. Client negotiates with wrong path
2. Server responds with new connection ID
3. Client tries to connect with malformed URL (`//iacmessagebus`)
4. Gets 301 redirect
5. Connection established but protocol mismatch
6. Server times out after 30 seconds (no valid SignalR messages)
7. Client reconnects → infinite loop

## Solution: Use SignalR Core Client

### ✅ Option 1: Use Modern Client (Recommended)

The server code already has the correct implementation! Use it as reference.

**New File: `clientsample/client_modern.go`**

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/mdaxf/iac-signalr/logger"
    "github.com/mdaxf/iac-signalr/signalr"
)

type MessageReceiver struct {
    signalr.Receiver
}

// Server can call these methods on the client
func (r *MessageReceiver) Receive(topic string, message string) {
    fmt.Printf("[Received] topic=%s, message=%s\n", topic, message)
}

func (r *MessageReceiver) Send(topic string, message string, connectionID string) {
    fmt.Printf("[Send] topic=%s, message=%s, from=%s\n", topic, message, connectionID)
}

func main() {
    // Server URL - MUST include the hub path
    serverURL := "http://127.0.0.1:8222/iacmessagebus"

    // Create logger
    ilog := logger.Log{ModuleName: logger.SignalR, User: "Client", ControllerName: "Go Client"}
    logAdapter := logger.NewSignalRLogAdapter(ilog)

    // Create client with WebSocket-only transport
    client, err := signalr.NewClient(
        context.Background(),
        signalr.WithReceiver(&MessageReceiver{}),
        signalr.WithConnector(func() (signalr.Connection, error) {
            ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
            defer cancel()

            // CRITICAL: Use WebSocket transport only!
            return signalr.NewHTTPConnection(ctx, serverURL,
                signalr.WithTransports(signalr.TransportWebSockets),
            )
        }),
        signalr.Logger(logAdapter, false),
        signalr.KeepAliveInterval(15*time.Second),  // Match server settings
        signalr.TimeoutInterval(60*time.Second),     // Match server settings
    )

    if err != nil {
        fmt.Printf("Failed to create client: %v\n", err)
        return
    }

    // Start the connection
    client.Start()
    fmt.Println("✅ Connected successfully!")

    // Send a message to the server
    _, err = client.Invoke("Send", "my-topic", "Hello from Go!", "client-123")
    if err != nil {
        fmt.Printf("Error: %v\n", err)
    }

    // Keep alive
    time.Sleep(60 * time.Second)

    // Cleanup
    client.Stop()
}
```

### Key Configuration Points

| Setting | Value | Reason |
|---------|-------|--------|
| **Server URL** | `http://127.0.0.1:8222/iacmessagebus` | MUST include hub path |
| **Transport** | `signalr.TransportWebSockets` | **WebSocket-only** to avoid fallback issues |
| **KeepAlive** | `15*time.Second` | Match server configuration |
| **Timeout** | `60*time.Second` | Match server configuration |
| **Connection Timeout** | `10*time.Second` | Initial connection timeout |

---

## Building and Running

### 1. **Build the Modern Client**

```bash
cd /home/user/iac-signalr/clientsample
go build -o client_modern client_modern.go
```

### 2. **Run the Client**

```bash
./client_modern
```

### 3. **Expected Output**

```
🚀 Starting SignalR Go Client...
📡 Connecting to: http://127.0.0.1:8222/iacmessagebus
✅ Client connected and started successfully!

📝 Subscribing to topic...
✅ Subscribed to topic: test-topic

📤 Starting to send messages...
✅ Sent message 1: Hello from Go client - Message #1 at 18:45:23
✅ Sent message 2: Hello from Go client - Message #2 at 18:45:33
...
```

---

## Migration Checklist

- [ ] **Replace old client code** with modern SignalR Core client
- [ ] **Use correct URL**: Include `/iacmessagebus` path
- [ ] **Force WebSocket transport**: `signalr.WithTransports(signalr.TransportWebSockets)`
- [ ] **Match server timeouts**: KeepAlive=15s, Timeout=60s
- [ ] **Implement receiver methods**: Define all methods server can call
- [ ] **Test connection stability**: Should stay connected indefinitely
- [ ] **Monitor logs**: Look for "connected and joining group" messages

---

## Comparison: Old vs New

### Old Client (`clientsample/client.go`)

```go
// ❌ Problems:
client := NewWebsocketClient()
err := client.Connect("http", "127.0.0.1:8222", []string{"iacmessagebus"})
// - Uses SignalR 1.x protocol
// - Wrong negotiation format
// - No proper keep-alive
// - Disconnects after 30s
```

### New Client (`client_modern.go`)

```go
// ✅ Correct:
client, err := signalr.NewClient(
    context.Background(),
    signalr.WithConnector(func() (signalr.Connection, error) {
        return signalr.NewHTTPConnection(ctx, "http://127.0.0.1:8222/iacmessagebus",
            signalr.WithTransports(signalr.TransportWebSockets),
        )
    }),
)
// - Uses SignalR Core protocol
// - Correct negotiation
// - WebSocket keep-alive
// - Stays connected indefinitely
```

---

## Testing Your Client

### 1. **Start the Server**

```bash
cd /home/user/iac-signalr
# Add to signalrconfig.json:
# "insecureSkipVerify": true,
# "timeoutInterval": 60

./iac-signalr
```

### 2. **Check Server Logs**

You should see:
```
[I] SignalR server configured - Transport: WebSocket-only, KeepAlive: 15s, Timeout: 60s
```

### 3. **Run Client**

```bash
cd clientsample
./client_modern
```

### 4. **Verify Connection**

**Server logs should show:**
```
[I] Client xyz123 connected and joining group IAC_Internal_MessageBus
... [no disconnection after 30 seconds] ✅
```

**Client logs should show:**
```
✅ Connected successfully!
✅ Sent message 1
✅ Sent message 2
... [stays connected] ✅
```

### 5. **Common Issues**

| Issue | Solution |
|-------|----------|
| "Connection refused" | Ensure server is running on correct port |
| "401 Unauthorized" | Add `insecureSkipVerify: true` to config |
| Still disconnecting | Check client uses `TransportWebSockets` |
| "negotiate failed" | Verify URL includes `/iacmessagebus` |

---

## Advanced: Custom Client Configuration

```go
// Configure custom timeouts
client, err := signalr.NewClient(
    context.Background(),
    signalr.WithConnector(func() (signalr.Connection, error) {
        ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
        defer cancel()

        return signalr.NewHTTPConnection(ctx, serverURL,
            signalr.WithTransports(signalr.TransportWebSockets),
            signalr.WithHTTPClient(&http.Client{
                Timeout: 30 * time.Second,
            }),
        )
    }),
    signalr.KeepAliveInterval(30*time.Second),  // Custom keep-alive
    signalr.TimeoutInterval(120*time.Second),    // Custom timeout
    signalr.HandshakeTimeout(20*time.Second),    // Custom handshake
)
```

---

## Summary

### What You Need to Change

1. ✅ **Use modern SignalR Core client** from `signalr` package
2. ✅ **Include hub path in URL**: `http://host:port/iacmessagebus`
3. ✅ **Force WebSocket transport**: Avoid SSE/long polling
4. ✅ **Match server timeouts**: 15s KeepAlive, 60s Timeout
5. ✅ **Implement receiver methods**: Define server callable methods

### What You Get

- ✅ **Stable connections** - No 30-second disconnections
- ✅ **Proper protocol** - SignalR Core compatible
- ✅ **Automatic keep-alive** - WebSocket ping/pong
- ✅ **Better performance** - Lower overhead
- ✅ **Easier debugging** - Clear error messages

### Files to Update

- 🔄 **Replace**: `clientsample/client.go` with `client_modern.go`
- 🔄 **Replace**: `client/client.go` with modern implementation
- ✅ **Server**: Already correct - no changes needed!

The auto-disconnect issue will be completely resolved with the modern client! 🎉
