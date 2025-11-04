package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/mdaxf/iac-signalr/logger"
)

func EnableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "http://127.0.0.1:8080")
	(*w).Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	(*w).Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, x-requested-with")
	(*w).Header().Set("Access-Control-Allow-Credentials", "true")
}

// LogRequests logs HTTP requests with structured logging and request correlation
func LogRequests(ilog logger.Log) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Generate request ID for correlation
			requestID := uuid.New().String()
			w.Header().Set("X-Request-ID", requestID)

			wrappedWriter := wrapResponseWriter(w)
			start := time.Now()

			EnableCors(&w)

			// Log incoming request
			ilog.Info(fmt.Sprintf("HTTP Request - requestID=%s method=%s uri=%s remoteAddr=%s",
				requestID, r.Method, r.URL.String(), r.RemoteAddr))

			h.ServeHTTP(wrappedWriter, r)

			status := wrappedWriter.status
			duration := time.Since(start)

			// Log completed request with appropriate level based on status code
			logMsg := fmt.Sprintf("HTTP Response - requestID=%s status=%d method=%s uri=%s duration=%v",
				requestID, status, r.Method, r.URL.String(), duration)

			if status >= 500 {
				ilog.Error(logMsg)
			} else if status >= 400 {
				ilog.Warn(logMsg)
			} else {
				ilog.Info(logMsg)
			}
		})
	}
}
