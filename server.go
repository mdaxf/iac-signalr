package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync"

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

var ilog logger.Log
var nodedata map[string]interface{}

var IACMessageBusName = "/iacmessagebus"
var SignalRConfig Config

func runHTTPServer(address string, hub signalr.HubInterface, clients string) {
	server, _ := signalr.NewServer(context.TODO(), signalr.SimpleHubFactory(hub),
		signalr.Logger(kitlog.NewLogfmtLogger(os.Stdout), false),
		signalr.KeepAliveInterval(10*time.Second), signalr.AllowOriginPatterns([]string{clients}),
		signalr.InsecureSkipVerify(true))

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
		if r.Header.Get("Authorization") != "apikey "+SignalRConfig.AppServer["apikey"].(string) {
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
		fmt.Errorf("failed to read configuration file: %v", err)
	}

	var config Config

	if err := json.Unmarshal(data, &config); err != nil {
		fmt.Errorf("failed to parse configuration file: %v", err)
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

		go runHTTPServer(address, hub, clients)
		<-time.After(time.Millisecond * 2)
		/*	go func() {
			fmt.Println(runHTTPClient(url, &receiver{}))
		}() */
	}()

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
	headers["Authorization"] = "apikey " + gconfig.AppServer["apikey"].(string)

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
		//	fmt.Error("Error in WebServiceCallFunc.Execute: %s", err)
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
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
		fmt.Println("Error getting hostname:", err)
		return nil, err
	}
	fmt.Println("Hostname: %s", hostname)

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		fmt.Println("Error getting IP addresses:", err)
		return nil, err
	}
	var ipnet *net.IPNet

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				fmt.Println("IPv4 address:", ipnet.IP.String())
			} else {
				fmt.Println("IPv6 address:", ipnet.IP.String())
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
