# Go Client Configuration Guide

## Problem with Current Clients

Your Go clients in `client/client.go` and `clientsample/client.go` are using **SignalR 1.x protocol**, which is incompatible with your **SignalR Core server**. This causes:

1. ❌ Wrong negotiation path (`/negotiate` instead of `/iacmessagebus/negotiate`)
2. ❌ Wrong protocol version
3. ❌ Different message format
4. ❌ No proper keep-alive handling
5. ❌ The 30-second disconnections you're seeing

## Solution: Use the Modern SignalR Client

The server already has the correct client implementation in `server.go:128-145`. Here's how to use it:

---

## ✅ **Correct Go Client Implementation**

### 1. **Updated Client Code (Recommended)**

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

// Receive handles messages from the server
func (r *MessageReceiver) Receive(topic string, message string) {
    fmt.Printf("Received: topic=%s, message=%s\n", topic, message)
}

// Send is a client method that can be called by the server
func (r *MessageReceiver) Send(topic string, message string, connectionID string) {
    fmt.Printf("Server called Send: topic=%s, message=%s, from=%s\n", topic, message, connectionID)
}

func main() {
    // Create logger adapter (optional - can use nil)
    ilog := logger.Log{ModuleName: logger.SignalR, User: "Client", ControllerName: "SignalR Client"}
    logAdapter := logger.NewSignalRLogAdapter(ilog)

    // Server address - IMPORTANT: Use full path with /iacmessagebus
    serverURL := "http://127.0.0.1:8222/iacmessagebus"

    // Create client with WebSocket-only transport
    client, err := signalr.NewClient(
        context.Background(),
        signalr.WithReceiver(&MessageReceiver{}),
        signalr.WithConnector(func() (signalr.Connection, error) {
            ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
            defer cancel()

            // Create connection with WebSocket transport
            return signalr.NewHTTPConnection(ctx, serverURL,
                signalr.WithTransports(signalr.TransportWebSockets), // WebSocket only!
            )
        }),
        signalr.Logger(logAdapter, false),
    )

    if err != nil {
        fmt.Printf("Failed to create client: %v\n", err)
        return
    }

    // Start the client
    client.Start()
    fmt.Println("✅ Client connected successfully!")

    // Keep client alive and send periodic messages
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for i := 0; i < 10; i++ {
        <-ticker.C

        // Call server hub method
        _, err := client.Invoke("Send", "test-topic", fmt.Sprintf("Message %d from Go client", i), "go-client-123")
        if err != nil {
            fmt.Printf("Error invoking Send: %v\n", err)
            break
        }
        fmt.Printf("Sent message %d\n", i)
    }

    // Graceful shutdown
    client.Stop()
    fmt.Println("Client disconnected")
}
```

### 2. **Key Changes from Legacy Clients**

| Aspect | ❌ Legacy Client | ✅ Modern Client |
|--------|-----------------|------------------|
| **Package** | Custom implementation | `github.com/mdaxf/iac-signalr/signalr` |
| **Protocol** | SignalR 1.x | SignalR Core |
| **Transport** | Mixed | **WebSocket-only** |
| **URL** | `http://host/negotiate` | `http://host/iacmessagebus` |
| **Keep-Alive** | Manual | **Automatic** (WebSocket ping/pong) |
| **Negotiation** | Old format | SignalR Core format |

---

## 📝 **Migration Steps**

### **Replace `clientsample/client.go`**

<function_calls>
<invoke name="Write">
<parameter name="content">package main

import (
	"context"
	"fmt"
	"time"

	"github.com/mdaxf/iac-signalr/logger"
	"github.com/mdaxf/iac-signalr/signalr"
)

// MessageReceiver handles messages from the SignalR server
type MessageReceiver struct {
	signalr.Receiver
}

// Receive handles the "receive" message from server
func (r *MessageReceiver) Receive(topic string, message string) {
	fmt.Printf("[Received] topic=%s, message=%s\n", topic, message)
}

// Send is called when server invokes "Send" on clients
func (r *MessageReceiver) Send(topic string, message string, connectionID string) {
	fmt.Printf("[Send] topic=%s, message=%s, from=%s\n", topic, message, connectionID)
}

// SendToBackEnd handles messages sent to backend
func (r *MessageReceiver) SendToBackEnd(jsonMsg map[string]interface{}) {
	fmt.Printf("[SendToBackEnd] message=%v\n", jsonMsg)
}

// Broadcast handles broadcast messages
func (r *MessageReceiver) Broadcast(message string) {
	fmt.Printf("[Broadcast] message=%s\n", message)
}

func main() {
	fmt.Println("🚀 Starting SignalR Go Client...")

	// Initialize logger
	ilog := logger.Log{
		ModuleName:     logger.SignalR,
		User:           "GoClient",
		ControllerName: "SignalR Client",
	}
	logger.Init(map[string]interface{}{
		"adapter": "console",
		"level":   "info",
	})

	// Create logger adapter
	logAdapter := logger.NewSignalRLogAdapter(ilog)

	// Server configuration
	serverURL := "http://127.0.0.1:8222/iacmessagebus"
	fmt.Printf("📡 Connecting to: %s\n", serverURL)

	// Create SignalR client with WebSocket-only transport
	client, err := signalr.NewClient(
		context.Background(),
		signalr.WithReceiver(&MessageReceiver{}),
		signalr.WithConnector(func() (signalr.Connection, error) {
			// Create connection context with timeout
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// Connect with WebSocket transport only
			return signalr.NewHTTPConnection(ctx, serverURL,
				signalr.WithTransports(signalr.TransportWebSockets),
			)
		}),
		signalr.Logger(logAdapter, false),
		signalr.KeepAliveInterval(15*time.Second),
		signalr.TimeoutInterval(60*time.Second),
	)

	if err != nil {
		fmt.Printf("❌ Failed to create client: %v\n", err)
		return
	}

	// Start the client connection
	client.Start()
	fmt.Println("✅ Client connected and started successfully!")

	// Wait a moment for connection to establish
	time.Sleep(2 * time.Second)

	// Example 1: Subscribe to a topic
	fmt.Println("\n📝 Subscribing to topic...")
	_, err = client.Invoke("Subscribe", "test-topic", "go-client-001")
	if err != nil {
		fmt.Printf("❌ Subscribe error: %v\n", err)
	} else {
		fmt.Println("✅ Subscribed to topic: test-topic")
	}

	// Example 2: Send messages periodically
	fmt.Println("\n📤 Starting to send messages...")
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for i := 0; i < 5; i++ {
		<-ticker.C

		message := fmt.Sprintf("Hello from Go client - Message #%d at %s",
			i+1, time.Now().Format("15:04:05"))

		// Invoke "Send" method on server hub
		_, err := client.Invoke("Send", "test-topic", message, "go-client-001")
		if err != nil {
			fmt.Printf("❌ Error sending message %d: %v\n", i+1, err)
			break
		}
		fmt.Printf("✅ Sent message %d: %s\n", i+1, message)
	}

	// Example 3: Broadcast a message
	fmt.Println("\n📢 Broadcasting message...")
	_, err = client.Invoke("Broadcast", "Broadcast from Go client!")
	if err != nil {
		fmt.Printf("❌ Broadcast error: %v\n", err)
	} else {
		fmt.Println("✅ Broadcast sent successfully")
	}

	// Keep connection alive for a bit longer
	fmt.Println("\n⏰ Keeping connection alive for 30 more seconds...")
	time.Sleep(30 * time.Second)

	// Graceful shutdown
	fmt.Println("\n🛑 Shutting down client...")
	client.Stop()
	fmt.Println("👋 Client disconnected gracefully")
}
