package middleware

import (
	"fmt"
	"net/http"
	"time"
)

// silentPaths are endpoints polled frequently by health monitors.
// Successful (2xx) requests to these paths are suppressed to avoid flooding logs.
var silentPaths = map[string]bool{
	"/":       true,
	"/health": true,
	"/status": true,
}

func EnableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "http://127.0.0.1:8080")
	(*w).Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	(*w).Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, x-requested-with")
	(*w).Header().Set("Access-Control-Allow-Credentials", "true")
}

// LogRequests writes simple request logs to STDOUT.
// Successful (2xx) requests to health-check paths (/, /health, /status) are
// suppressed to avoid flooding logs with heartbeat noise.
// Non-2xx responses are always logged so errors are visible.
func LogRequests(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrappedWriter := wrapResponseWriter(w)
		start := time.Now()
		EnableCors(&w)
		h.ServeHTTP(wrappedWriter, r)

		status := wrappedWriter.status
		duration := time.Since(start)

		// Suppress successful health-check noise; always surface errors
		if silentPaths[r.URL.Path] && status >= 200 && status < 300 {
			return
		}

		fmt.Printf("%03d %s %s %v\n", status, r.Method, r.URL.String(), duration)
	})
}
