package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"

	//	"log"

	//	"strings"

	//	"net"
	"net/http"
	"os"
	"time"

	//	"github.com/rs/cors"

	kitlog "github.com/go-kit/log"
	"github.com/google/uuid"

	"github.com/mdaxf/iac-signalr/logger"
	"github.com/mdaxf/iac-signalr/middleware"
	"github.com/mdaxf/iac-signalr/public"
	"github.com/mdaxf/iac-signalr/signalr"
	"github.com/mdaxf/iac/health"
)

type Config struct {
	Address   string                 `json:"address"`
	Clients   string                 `json:"clients"`
	AppServer map[string]interface{} `json:"appserver"`
	Log       map[string]interface{} `json:"log"`
}

// HealthStatus represents the overall health status of the server
type HealthStatus struct {
	Status            string                 `json:"status"` // "healthy", "degraded", "unhealthy"
	Timestamp         time.Time              `json:"timestamp"`
	Node              map[string]interface{} `json:"node"`
	Uptime            string                 `json:"uptime"`
	ActiveConnections int                    `json:"activeConnections"`
	TotalConnections  uint64                 `json:"totalConnections"`
	Checks            map[string]CheckResult `json:"checks"`
}

// CheckResult represents the result of a single health check
type CheckResult struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

var ilog logger.Log
var nodedata map[string]interface{}
var globalHub *IACMessageBus // Store reference to hub for health checks

var IACMessageBusName = "/iacmessagebus"
var SignalRConfig Config

func runHTTPServer(address string, hub signalr.HubInterface, clients string) *http.Server {
	signalrServer, _ := signalr.NewServer(context.TODO(), signalr.SimpleHubFactory(hub),
		signalr.Logger(kitlog.NewLogfmtLogger(os.Stdout), false),
		signalr.KeepAliveInterval(10*time.Second), signalr.AllowOriginPatterns([]string{clients}),
		signalr.InsecureSkipVerify(true))

	signalr.AllowedClients = clients

	router := http.NewServeMux()

	signalrServer.MapHTTP(signalr.WithHTTPServeMux(router), IACMessageBusName)

	ilog.Info(fmt.Sprintf("Serving public content from the embedded filesystem\n"))
	router.Handle("/", http.FileServer(http.FS(public.FS)))

	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Optional authentication - only enforce if Authorization header is present
		if auth := r.Header.Get("Authorization"); auth != "" {
			if auth != "apikey "+SignalRConfig.AppServer["apikey"].(string) {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		// Build health status response
		healthStatus := HealthStatus{
			Status:    "healthy",
			Timestamp: time.Now().UTC(),
			Node:      nodedata,
			Checks:    make(map[string]CheckResult),
		}

		// Add uptime
		if startTime, ok := nodedata["StartTime"].(time.Time); ok {
			healthStatus.Uptime = time.Since(startTime).String()
		}

		// Add connection metrics if hub is available
		if globalHub != nil {
			healthStatus.ActiveConnections = globalHub.GetConnectionCount()
			healthStatus.TotalConnections = globalHub.GetTotalConnections()
		}

		// Check SignalR server status
		result, err := CheckServiceStatus(ilog, SignalRConfig)
		if err != nil {
			healthStatus.Status = "degraded"
			healthStatus.Checks["signalr"] = CheckResult{
				Status:  "unhealthy",
				Message: err.Error(),
			}
			ilog.Warn(fmt.Sprintf("Health check - SignalR server check failed: %v", err))
		} else {
			healthStatus.Checks["signalr"] = CheckResult{Status: "healthy"}
		}

		// Check app server connectivity
		if err := checkAppServerConnectivity(SignalRConfig); err != nil {
			healthStatus.Status = "degraded"
			healthStatus.Checks["appserver"] = CheckResult{
				Status:  "unhealthy",
				Message: err.Error(),
			}
			ilog.Warn(fmt.Sprintf("Health check - AppServer connectivity failed: %v", err))
		} else {
			healthStatus.Checks["appserver"] = CheckResult{Status: "healthy"}
		}

		// Determine HTTP status code based on health status
		statusCode := http.StatusOK
		if healthStatus.Status == "unhealthy" {
			statusCode = http.StatusServiceUnavailable
		}

		// Send response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(healthStatus)
	})

	// Create HTTP server with explicit configuration
	srv := &http.Server{
		Addr:         address,
		Handler:      middleware.LogRequests(ilog)(router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ilog.Info(fmt.Sprintf("HTTP server configured - address=%s readTimeout=15s writeTimeout=15s idleTimeout=60s", address))

	return srv
}

func runHTTPClient(address string, receiver interface{}) error {
	c, err := signalr.NewClient(context.Background(), nil,
		signalr.WithReceiver(receiver),
		signalr.WithConnector(func() (signalr.Connection, error) {
			creationCtx, _ := context.WithTimeout(context.Background(), 2*time.Second)
			return signalr.NewHTTPConnection(creationCtx, address)
		}),
		signalr.Logger(kitlog.NewLogfmtLogger(os.Stdout), false))
	if err != nil {
		return err
	}
	c.Start()

	fmt.Println("Client started")

	return nil
}

type receiver struct {
	signalr.Receiver
}

func (r *receiver) Receive(msg string) {
	fmt.Println(msg)
	// The silly client urges the server to end his connection after 10 seconds
	r.Server().Send("abort")
}

func main() {
	appconfig := "signalrconfig.json"
	data, err := ioutil.ReadFile(appconfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Failed to read configuration file '%s': %v\n", appconfig, err)
		os.Exit(1)
	}

	var config Config

	if err := json.Unmarshal(data, &config); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Failed to parse configuration file '%s': %v\n", appconfig, err)
		os.Exit(1)
	}
	SignalRConfig = config
	address := config.Address

	clients := config.Clients

	nodedata = make(map[string]interface{})
	nodedata["Name"] = "iac-signalr"
	nodedata["AppID"] = uuid.New().String()
	nodedata["Description"] = "IAC SignalR Server"
	nodedata["Type"] = "SignalR Server"
	nodedata["Version"] = "1.0.0"
	nodedata["Status"] = "Running"
	nodedata["StartTime"] = time.Now().UTC()

	// Initialize global logger with user mdaxf/iac for proper logging attribution
	ilog = logger.Log{ModuleName: logger.SignalR, User: "mdaxf/iac", ControllerName: "Signalr Server"}
	logger.Init(config.Log)

	// Validate configuration
	if err := validateConfig(&config); err != nil {
		ilog.Critical(fmt.Sprintf("FATAL: Invalid configuration: %v", err))
		os.Exit(1)
	}

	ilog.Info(fmt.Sprintf("Starting SignalR Server Address: %s, allow Clients: %s", address, clients))

	// Create the hub
	hub := &IACMessageBus{
		ilog: ilog,
	}

	// Store hub reference for health checks
	globalHub = hub

	// Create HTTP server
	srv := runHTTPServer(address, hub, clients)

	hip, err := GetHostandIPAddress()

	if err != nil {
		ilog.Error(fmt.Sprintf("Failed to get host and ip address: %v", err))
	}
	for key, value := range hip {
		nodedata[key] = value
	}
	if hip["Host"] != nil {
		port := 0
		if strings.Contains(address, ":") {
			urls := strings.Split(address, ":")
			if len(urls) == 2 {
				p, err := strconv.Atoi(urls[1])
				if err != nil {
					ilog.Error(fmt.Sprintf("Failed to get port number: %v", err))
				} else {
					port = p
				}

			}
		} else {
			p, err := strconv.Atoi(address)
			if err != nil {
				ilog.Error(fmt.Sprintf("Failed to get port number: %v", err))
			} else {
				port = p
			}

		}

		if port > 0 {
			nodedata["healthapi"] = fmt.Sprintf("http://%s:%d/health", hip["Host"], port)
		}
	}
	// Start HTTP server in goroutine
	go func() {
		ilog.Info(fmt.Sprintf("Starting HTTP server on %s", address))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			ilog.Error(fmt.Sprintf("HTTP server error: %v", err))
			os.Exit(1)
		}
	}()

	// Start the heartbeat in goroutine
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				HeartBeat(ilog, config)
			case <-heartbeatDone:
				ilog.Info("Heartbeat stopped")
				return
			}
		}
	}()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Wait for interrupt signal
	sig := <-sigChan
	ilog.Info(fmt.Sprintf("Received signal: %v, initiating graceful shutdown", sig))

	// Stop heartbeat
	close(heartbeatDone)

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Attempt graceful shutdown
	if err := srv.Shutdown(shutdownCtx); err != nil {
		ilog.Error(fmt.Sprintf("Server forced to shutdown: %v", err))
		os.Exit(1)
	}

	ilog.Info("Server exited gracefully")
}

func HeartBeat(ilog logger.Log, gconfig Config) {
	// Reduced heartbeat debug logging to minimize log noise
	// ilog.Debug("Start HeartBeat for iac-activemq application with appid: " + nodedata["AppID"].(string))
	appHeartBeatUrl := gconfig.AppServer["url"].(string) + "/IACComponents/heartbeat"
	//ilog.Debug("HeartBeat URL: " + appHeartBeatUrl)

	result, err := CheckServiceStatus(ilog, gconfig)
	if err != nil {
		ilog.Error(fmt.Sprintf("HeartBeat error: %v", err))
	}

	data := make(map[string]interface{})
	data["Node"] = nodedata
	data["Result"] = result
	data["ServiceStatus"] = make(map[string]interface{})
	data["timestamp"] = time.Now().UTC()
	// send the heartbeat to the server
	headers := make(map[string]string)
	headers["Content-Type"] = "application/json"
	headers["Authorization"] = "apikey " + gconfig.AppServer["apikey"].(string)

	// Create context with timeout for heartbeat request
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = CallWebService(ctx, appHeartBeatUrl, "POST", data, headers, 30*time.Second)

	if err != nil {
		ilog.Error(fmt.Sprintf("HeartBeat error: %v", err))
		return
	}

	//	ilog.Debug(fmt.Sprintf("HeartBeat post response: %v", response))
}

func CheckServiceStatus(iLog logger.Log, config Config) (map[string]interface{}, error) {
	// Reduced heartbeat debug logging to minimize log noise
	// iLog.Debug("Check SignalR Server Status")

	result, err := health.CheckSignalRServerHealth(nodedata, "http://"+config.Address, "ws:"+config.Address)
	if err != nil {
		iLog.Error(fmt.Sprintf("Check SignalR Server Status error: %v", err))
		return nil, err
	}

	return result, nil
}

func CallWebService(ctx context.Context, url string, method string, data map[string]interface{}, headers map[string]string, timeout time.Duration) (map[string]interface{}, error) {
	var result map[string]interface{}

	// Create context with timeout if not already set
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: timeout,
	}

	bytesdata, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request data: %w", err)
	}

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(bytesdata))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	if headers != nil {
		for key, value := range headers {
			req.Header.Set(key, value)
		}
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		// Check for context deadline exceeded
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("request timeout exceeded: %w", err)
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check HTTP status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read response body
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Unmarshal response
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// checkAppServerConnectivity checks if the app server is reachable
func checkAppServerConnectivity(config Config) error {
	if config.AppServer == nil || config.AppServer["url"] == nil {
		return fmt.Errorf("app server not configured")
	}

	url := config.AppServer["url"].(string)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	// Any response is considered success (server is reachable)
	return nil
}

// validateConfig validates the configuration and returns an error if invalid
func validateConfig(config *Config) error {
	if config.Address == "" {
		return fmt.Errorf("address is required")
	}
	if config.AppServer == nil {
		return fmt.Errorf("appserver configuration is required")
	}
	if config.AppServer["url"] == nil || config.AppServer["url"] == "" {
		return fmt.Errorf("appserver.url is required")
	}
	if config.AppServer["apikey"] == nil || config.AppServer["apikey"] == "" {
		return fmt.Errorf("appserver.apikey is required")
	}
	if config.Clients == "" {
		return fmt.Errorf("clients configuration is required")
	}
	return nil
}

func GetHostandIPAddress() (map[string]interface{}, error) {
	hostname, err := os.Hostname()
	if err != nil {
		ilog.Error(fmt.Sprintf("Error getting hostname: %v", err))
		return nil, err
	}
	ilog.Info(fmt.Sprintf("Hostname: %s", hostname))

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		ilog.Error(fmt.Sprintf("Error getting IP addresses: %v", err))
		return nil, err
	}
	var ipnet *net.IPNet

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				ilog.Debug(fmt.Sprintf("Network interface found - type=IPv4 address=%s", ipnet.IP.String()))
			} else {
				ilog.Debug(fmt.Sprintf("Network interface found - type=IPv6 address=%s", ipnet.IP.String()))
			}
		}
	}

	osName := runtime.GOOS

	nodedata := make(map[string]interface{})
	nodedata["Host"] = hostname
	nodedata["OS"] = osName
	nodedata["IPAddress"] = ipnet.IP.String()

	return nodedata, nil
}
