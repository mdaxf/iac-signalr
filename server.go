package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/mdaxf/iac-signalr/logger"
	"github.com/mdaxf/iac-signalr/middleware"
	"github.com/mdaxf/iac-signalr/public"
	"github.com/mdaxf/iac-signalr/signalr"
	"github.com/mdaxf/iac/health"
)

type Config struct {
	Address            string                 `json:"address"`
	Clients            string                 `json:"clients"`
	AppServer          map[string]interface{} `json:"appserver"`
	Log                map[string]interface{} `json:"log"`
	InsecureSkipVerify bool                   `json:"insecureSkipVerify"`
}

var ilog logger.Log
var nodedata map[string]interface{}

var IACMessageBusName = "/iacmessagebus"
var SignalRConfig Config

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getAPIKey retrieves API key from environment variable or config
func getAPIKey(config Config) string {
	// First try environment variable for security
	if apiKey := os.Getenv("SIGNALR_API_KEY"); apiKey != "" {
		return apiKey
	}
	// Fall back to config (not recommended for production)
	if config.AppServer != nil {
		if apiKey, ok := config.AppServer["apikey"].(string); ok {
			return apiKey
		}
	}
	return ""
}

// secureCompare performs constant-time comparison to prevent timing attacks
func secureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func runHTTPServer(address string, hub signalr.HubInterface, clients string, insecureSkipVerify bool) {
	// Create SignalR logger adapter
	logAdapter := logger.NewSignalRLogAdapter(ilog)

	// Configure server with proper timeout settings
	// TimeoutInterval should be at least 2x KeepAliveInterval
	server, err := signalr.NewServer(context.TODO(), signalr.SimpleHubFactory(hub),
		signalr.Logger(logAdapter, false),
		signalr.KeepAliveInterval(15*time.Second),
		signalr.TimeoutInterval(30*time.Second),
		signalr.HandshakeTimeout(15*time.Second),
		signalr.AllowOriginPatterns([]string{clients}),
		signalr.InsecureSkipVerify(insecureSkipVerify))

	if err != nil {
		ilog.Error(fmt.Sprintf("Failed to create SignalR server: %v", err))
		return
	}

	ilog.Info(fmt.Sprintf("SignalR server configured - KeepAlive: 15s, Timeout: 30s, InsecureSkipVerify: %v", insecureSkipVerify))

	signalr.AllowedClients = clients

	router := http.NewServeMux()

	server.MapHTTP(signalr.WithHTTPServeMux(router), IACMessageBusName)

	ilog.Info(fmt.Sprintf("Serving public content from the embedded filesystem\n"))
	router.Handle("/", http.FileServer(http.FS(public.FS)))

	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Secure API key comparison using constant time comparison
		expectedAuth := "apikey " + getAPIKey(SignalRConfig)
		actualAuth := r.Header.Get("Authorization")
		if !secureCompare(expectedAuth, actualAuth) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		result, err := CheckServiceStatus(ilog, SignalRConfig)
		if err != nil {
			ilog.Error(fmt.Sprintf("HeartBeat error: %v", err))
		}

		data := make(map[string]interface{})
		data["Node"] = nodedata
		data["Result"] = result
		data["ServiceStatus"] = make(map[string]interface{})
		data["timestamp"] = time.Now().UTC()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	})

	ilog.Info(fmt.Sprintf("Listening for websocket connections on %s %s", "Address:", address))
	//	fmt.Printf("Listening for websocket connections on http://%s\n", address)
	if err := http.ListenAndServe(address, middleware.LogRequests(router)); err != nil {
		ilog.Error(fmt.Sprintf("ListenAndServe: %s", err))
	}
}

func runHTTPClient(address string, receiver interface{}, logAdapter *logger.SignalRLogAdapter) error {
	c, err := signalr.NewClient(context.Background(), nil,
		signalr.WithReceiver(receiver),
		signalr.WithConnector(func() (signalr.Connection, error) {
			creationCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			return signalr.NewHTTPConnection(creationCtx, address)
		}),
		signalr.Logger(logAdapter, false))
	if err != nil {
		return err
	}
	c.Start()

	ilog.Info("SignalR client started")

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
	// Support environment variable for config file path
	appconfig := getEnv("SIGNALR_CONFIG", "signalrconfig.json")

	data, err := os.ReadFile(appconfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read configuration file '%s': %v\n", appconfig, err)
		os.Exit(1)
	}

	var config Config

	if err := json.Unmarshal(data, &config); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse configuration file: %v\n", err)
		os.Exit(1)
	}

	// Override config with environment variables if set
	if envAddress := os.Getenv("SIGNALR_ADDRESS"); envAddress != "" {
		config.Address = envAddress
	}
	if envClients := os.Getenv("SIGNALR_CLIENTS"); envClients != "" {
		config.Clients = envClients
	}
	if envInsecure := os.Getenv("SIGNALR_INSECURE_SKIP_VERIFY"); envInsecure == "true" {
		config.InsecureSkipVerify = true
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

	ilog := logger.Log{ModuleName: logger.SignalR, User: "System", ControllerName: "Signalr Server"}
	logger.Init(config.Log)

	ilog.Info(fmt.Sprintf("Starting SignalR Server Address: %s, allow Clients: %s", address, clients))

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		//	url := "http://" + address + IACMessageBusName
		hub := &IACMessageBus{
			ilog: ilog,
		}

		go runHTTPServer(address, hub, clients, config.InsecureSkipVerify)
		<-time.After(time.Millisecond * 2)
		/*	go func() {
			fmt.Println(runHTTPClient(url, &receiver{}))
		}() */
	}()
	/*
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
		// Start the heartbeat
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				HeartBeat(ilog, config)
				time.Sleep(5 * time.Minute)
			}
		}()
	*/
	wg.Wait()

	ch := make(chan struct{})
	<-ch
}

func HeartBeat(ilog logger.Log, gconfig Config) {
	ilog.Debug("Start HeartBeat for iac-activemq application with appid: " + nodedata["AppID"].(string))
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
	headers["Authorization"] = "apikey " + getAPIKey(gconfig)

	_, err = CallWebService(appHeartBeatUrl, "POST", data, headers)

	if err != nil {
		ilog.Error(fmt.Sprintf("HeartBeat error: %v", err))
		return
	}

	//	ilog.Debug(fmt.Sprintf("HeartBeat post response: %v", response))
}

func CheckServiceStatus(iLog logger.Log, config Config) (map[string]interface{}, error) {
	iLog.Debug("Check SignalR Server Status")

	result, err := health.CheckSignalRServerHealth(nodedata, "http://"+config.Address, "ws:"+config.Address)
	if err != nil {
		iLog.Error(fmt.Sprintf("Check SignalR Server Status error: %v", err))
		return nil, err
	}

	return result, nil
}

func CallWebService(url string, method string, data map[string]interface{}, headers map[string]string) (map[string]interface{}, error) {
	var result map[string]interface{}
	// Create a new HTTP client
	client := &http.Client{}

	bytesdata, err := json.Marshal(data)
	if err != nil {
		//	fmt.Error(fmt.Sprintf("Error:", err))
		return nil, err
	}

	req, err := http.NewRequest(method, url, bytes.NewBuffer(bytesdata))

	if err != nil {
		//	fmt.Error("Error in WebServiceCallFunc.Execute: %s", err)
		return nil, err
	}
	if headers != nil {
		for key, value := range headers {
			req.Header.Set(key, value)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	err = json.Unmarshal(respBody, &result)
	if err != nil {
		//	fmt.Error(fmt.Sprintf("Error:", err))
		return nil, err
	}
	//	fmt.printf("Response data: %v", result)
	return result, nil
}
func GetHostandIPAddress() (map[string]interface{}, error) {
	hostname, err := os.Hostname()
	if err != nil {
		ilog.Error(fmt.Sprintf("Error getting hostname: %v", err))
		return nil, err
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		ilog.Error(fmt.Sprintf("Error getting IP addresses: %v", err))
		return nil, err
	}

	var ipAddress string
	// Find first non-loopback IPv4 address
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				ipAddress = ipnet.IP.String()
				ilog.Debug(fmt.Sprintf("Found IPv4 address: %s", ipAddress))
				break
			}
		}
	}

	// If no IPv4 found, try IPv6
	if ipAddress == "" {
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				ipAddress = ipnet.IP.String()
				ilog.Debug(fmt.Sprintf("Found IPv6 address: %s", ipAddress))
				break
			}
		}
	}

	// Default to localhost if no address found
	if ipAddress == "" {
		ipAddress = "127.0.0.1"
		ilog.Warn("No network address found, defaulting to 127.0.0.1")
	}

	osName := runtime.GOOS

	nodedata := make(map[string]interface{})
	nodedata["Host"] = hostname
	nodedata["OS"] = osName
	nodedata["IPAddress"] = ipAddress

	return nodedata, nil
}
