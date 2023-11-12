package middleware

import (
	"fmt"
	"net/http"
	"time"
)

func EnableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "http://127.0.0.1:8080")
	(*w).Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	(*w).Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, x-requested-with")
	(*w).Header().Set("Access-Control-Allow-Credentials", "true")
}

// LogRequests writes simple request logs to STDOUT so that we can see what requests the server is handling
func LogRequests(h http.Handler) http.Handler {
	// type our middleware as an http.HandlerFunc so that it is seen as an http.Handler
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// wrap the original response writer so we can capture response details
		wrappedWriter := wrapResponseWriter(w)
		start := time.Now() // request start time
		EnableCors(&w)
		// serve the inner request
		h.ServeHTTP(wrappedWriter, r)

		// extract request/response details
		status := wrappedWriter.status
		uri := r.URL.String()
		method := r.Method
		duration := time.Since(start)

		fmt.Println(w.Header(), r.Header, r.Body)
		// write to console
		fmt.Printf("%03d %s %s %v \n", status, method, uri, duration)
	})
}
