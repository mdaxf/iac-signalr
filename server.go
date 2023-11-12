package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io/ioutil"

	//	"log"

	//	"strings"

	//	"net"
	"net/http"
	"os"
	"time"

	//	"github.com/rs/cors"

	kitlog "github.com/go-kit/log"

	"github.com/mdaxf/iac-signalr/logger"
	"github.com/mdaxf/iac-signalr/middleware"
	"github.com/mdaxf/iac-signalr/public"
	"github.com/mdaxf/iac-signalr/signalr"
)

type Config struct {
	Address string                 `json:"address"`
	Clients string                 `json:"clients"`
	Log     map[string]interface{} `json:"log"`
}

var ilog logger.Log

var IACMessageBusName = "/iacmessagebus"

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

	address := config.Address

	clients := config.Clients

	ilog := logger.Log{ModuleName: logger.SignalR, User: "System", ControllerName: "Signalr Server"}
	logger.Init(config.Log)

	ilog.Info(fmt.Sprintf("Starting SignalR Server Address: %s, allow Clients: %s", address, clients))

	//	url := "http://" + address + IACMessageBusName
	hub := &IACMessageBus{
		ilog: ilog,
	}

	go runHTTPServer(address, hub, clients)
	<-time.After(time.Millisecond * 2)
	/*	go func() {
		fmt.Println(runHTTPClient(url, &receiver{}))
	}() */
	ch := make(chan struct{})
	<-ch
}
