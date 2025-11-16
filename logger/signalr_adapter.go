package logger

import (
	"fmt"
	"strings"
)

// SignalRLogAdapter adapts the go-kit logger interface to our custom logger
type SignalRLogAdapter struct {
	log Log
}

// NewSignalRLogAdapter creates a new adapter for SignalR logging
func NewSignalRLogAdapter(log Log) *SignalRLogAdapter {
	return &SignalRLogAdapter{log: log}
}

// Log implements the go-kit logger interface
func (l *SignalRLogAdapter) Log(keyVals ...interface{}) error {
	// Convert keyVals to a map for easy lookup
	m := make(map[string]interface{})
	for i := 0; i < len(keyVals); i += 2 {
		if i+1 < len(keyVals) {
			k := fmt.Sprintf("%v", keyVals[i])
			m[k] = keyVals[i+1]
		}
	}

	// Extract level if present
	level := ""
	if v, ok := m["level"]; ok {
		level = strings.ToLower(fmt.Sprintf("%v", v))
	}

	// Rebuild a clean structured message
	var sb strings.Builder
	for k, v := range m {
		if k != "level" { // Don't duplicate level in message
			sb.WriteString(fmt.Sprintf("%s=%v ", k, v))
		}
	}
	msg := strings.TrimSpace(sb.String())

	// Route to correct log category based on level
	switch level {
	case "debug":
		// Filter out excessive debug logs in production
		return nil
	case "warn", "warning":
		l.log.Warn(msg)
	case "error":
		l.log.Error(msg)
	case "info":
		l.log.Info(msg)
	default:
		// For messages without level or unknown level
		if msg != "" {
			l.log.Info(msg)
		}
	}

	return nil
}
