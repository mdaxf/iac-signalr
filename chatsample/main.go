package main

import (
	"context"
	_ "embed"
	"fmt"

	//	"log"
	//	"net/http"
	"os"
	"strings"
	"time"

	kitlog "github.com/go-kit/log"

	//	"github.com/mdaxf/iac-signalr/middleware"
	//	"github.com/mdaxf/iac-signalr/public"
	"github.com/mdaxf/iac-signalr/signalr"
)

type chat struct {
	signalr.Hub
}

func (c *chat) OnConnected(connectionID string) {
	fmt.Printf("%s connected\n", connectionID)
	c.Groups().AddToGroup("group", connectionID)
}

func (c *chat) OnDisconnected(connectionID string) {
	fmt.Printf("%s disconnected\n", connectionID)
	c.Groups().RemoveFromGroup("group", connectionID)
}

func (c *chat) Broadcast(message string) {
	// Broadcast to all clients
	c.Clients().Group("group").Send("receive", message)
}

func (c *chat) Echo(message string) {
	c.Clients().Caller().Send("receive", message)
}

func (c *chat) Panic() {
	panic("Don't panic!")
}

func (c *chat) RequestAsync(message string) <-chan map[string]string {
	r := make(chan map[string]string)
	go func() {
		defer close(r)
		time.Sleep(4 * time.Second)
		m := make(map[string]string)
		m["ToUpper"] = strings.ToUpper(message)
		m["ToLower"] = strings.ToLower(message)
		m["len"] = fmt.Sprint(len(message))
		r <- m
	}()
	return r
}

func (c *chat) RequestTuple(message string) (string, string, int) {
	return strings.ToUpper(message), strings.ToLower(message), len(message)
}

func (c *chat) DateStream() <-chan string {
	r := make(chan string)
	go func() {
		defer close(r)
		for i := 0; i < 50; i++ {
			r <- fmt.Sprint(time.Now().Clock())
			time.Sleep(time.Second)
		}
	}()
	return r
}

func (c *chat) UploadStream(upload1 <-chan int, factor float64, upload2 <-chan float64) {
	ok1 := true
	ok2 := true
	u1 := 0
	u2 := 0.0
	c.Echo(fmt.Sprintf("f: %v", factor))
	for {
		select {
		case u1, ok1 = <-upload1:
			if ok1 {
				c.Echo(fmt.Sprintf("u1: %v", u1))
			} else if !ok2 {
				c.Echo("Finished")
				return
			}
		case u2, ok2 = <-upload2:
			if ok2 {
				c.Echo(fmt.Sprintf("u2: %v", u2))
			} else if !ok1 {
				c.Echo("Finished")
				return
			}
		}
	}
}

func (c *chat) Abort() {
	fmt.Println("Abort")
	c.Hub.Abort()
}

//func runTCPServer(address string, hub signalr.HubInterface) {
//	listener, err := net.Listen("tcp", address)
//
//	if err != nil {
//		fmt.Println(err)
//		return
//	}
//
//	fmt.Printf("Listening for TCP connection on %s\n", listener.Addr())
//
//	server, _ := signalr.NewServer(context.TODO(), signalr.UseHub(hub))
//
//	for {
//		conn, err := listener.Accept()
//
//		if err != nil {
//			fmt.Println(err)
//			break
//		}
//
//		go server.Serve(context.TODO(), newNetConnection(conn))
//	}
//}
/*
func runHTTPServer(address string, hub signalr.HubInterface) {
	server, _ := signalr.NewServer(context.TODO(), signalr.SimpleHubFactory(hub),
		signalr.Logger(kitlog.NewLogfmtLogger(os.Stdout), false),
		signalr.KeepAliveInterval(2*time.Second))
	router := http.NewServeMux()
	server.MapHTTP(signalr.WithHTTPServeMux(router), "/chat")

	fmt.Printf("Serving public content from the embedded filesystem\n")
	router.Handle("/", http.FileServer(http.FS(public.FS)))
	fmt.Printf("Listening for websocket connections on http://%s\n", address)
	if err := http.ListenAndServe(address, middleware.LogRequests(router)); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
*/

var Client signalr.Client

func runHTTPClient(address string, receiver interface{}) error {
	c, err := signalr.NewClient(context.Background(), nil,
		signalr.WithReceiver(receiver),
		signalr.WithConnector(func() (signalr.Connection, error) {
			creationCtx, _ := context.WithTimeout(context.Background(), 2*time.Second)
			return signalr.NewHTTPConnection(creationCtx, address)
		}),
		signalr.Logger(kitlog.NewLogfmtLogger(os.Stdout), true))
	if err != nil {
		return err
	}
	c.Start()

	fmt.Println("Client started")
	c.Invoke("send", "broadcast", "Hello world!", "")
	<-time.After(time.Second)

	c.Invoke("send", "echo", "Hello world!", "")
	<-time.After(time.Second)

	c.Invoke("send", "Test", "this is a message from the GO client", "")
	<-time.After(time.Second)

	Client = c

	return nil
}

type broadcast struct {
	signalr.Receiver
}

func (r *broadcast) BroadCast(msg string) {
	fmt.Println(msg)
	// The silly client urges the server to end his connection after 10 seconds
	r.Server().Send("abort")
}

func main() {
	//	hub := &chat{}

	//go runTCPServer("127.0.0.1:8007", hub)
	//	go runHTTPServer("localhost:8086", hub)
	//	<-time.After(time.Millisecond * 2)
	go func() {
		//	fmt.Println(runHTTPClient("http://localhost:8222/iacmessagebus", &broadcast{}))
		fmt.Println(runHTTPClient("http://localhost:8222/iacmessagebus", &IACMessageBus{}))
		i := 0
		for i < 10 {
			fmt.Println("Sending message ", i)
			Client.Invoke("send", "Test", fmt.Sprintf("this is a message from the GO client %d", i), "")
			<-time.After(time.Second)
			i++
		}
	}()
	ch := make(chan struct{})
	<-ch

}

type IACMessageBus struct {
	signalr.Hub
}

var groupname = "IAC_Internal_MessageBus"

func (c *IACMessageBus) Receive(message string) {
	fmt.Printf("Receive message: %s \n", message)
}

/*
func (c *IACMessageBus) Subscribe(topic string, connectionID string) {
	fmt.Printf("Subscribe: topic: %s, sender: %s\n", topic, connectionID)
}
func (c *IACMessageBus) Send(topic string, message string, connectionID string) {
	fmt.Printf("Send: topic: %s, message: %s, sender: %s\n", topic, message, connectionID)
	c.Clients().Group(groupname).Send(topic, message)
	//	c.Clients().Caller().Send("receive", message)
}

func (c *IACMessageBus) AddMessage(message string, topic string, sender string) {
	fmt.Printf("AddMessage: topic: %s, message: %s, sender: %s\n", topic, message, sender)
	c.Clients().Group(groupname).Send(topic, message)
}

// add the client to the connection
func (c *IACMessageBus) OnConnected(connectionID string) {
	fmt.Printf("%s connected\n", connectionID)
	c.Groups().AddToGroup(groupname, connectionID)
	fmt.Printf("%s connected and added to group %s\n", connectionID, groupname)
}

func (c *IACMessageBus) OnDisconnected(connectionID string) {
	fmt.Printf("%s disconnected\n", connectionID)
	c.Groups().RemoveFromGroup(groupname, connectionID)
	fmt.Printf("%s disconnected and removed from group %s\n", connectionID, groupname)
}

func (c *IACMessageBus) Broadcast(message string) {
	// Broadcast to all clients
	fmt.Printf("broadcast message: %s\n", message)
	c.Clients().Group(groupname).Send("broadcast", message)
	//c.Clients().Group(groupname).Send("receive", message)
}

func (c *IACMessageBus) Echo(message string) {
	c.Clients().Caller().Send("echo", message)
	//	c.Clients().Caller().Send("receive", message)
}

func (c *IACMessageBus) Panic() {
	panic("Don't panic!")
}

func (c *IACMessageBus) RequestAsync(message string) <-chan map[string]string {
	r := make(chan map[string]string)
	go func() {
		defer close(r)
		time.Sleep(4 * time.Second)
		m := make(map[string]string)
		m["ToUpper"] = strings.ToUpper(message)
		m["ToLower"] = strings.ToLower(message)
		m["len"] = fmt.Sprint(len(message))
		r <- m
	}()
	return r
}

func (c *IACMessageBus) RequestTuple(message string) (string, string, int) {
	return strings.ToUpper(message), strings.ToLower(message), len(message)
}

func (c *IACMessageBus) DateStream() <-chan string {
	r := make(chan string)
	go func() {
		defer close(r)
		for i := 0; i < 50; i++ {
			r <- fmt.Sprint(time.Now().Clock())
			time.Sleep(time.Second)
		}
	}()
	return r
}

func (c *IACMessageBus) UploadStream(upload1 <-chan int, factor float64, upload2 <-chan float64) {
	ok1 := true
	ok2 := true
	u1 := 0
	u2 := 0.0
	c.Echo(fmt.Sprintf("f: %v", factor))
	for {
		select {
		case u1, ok1 = <-upload1:
			if ok1 {
				c.Echo(fmt.Sprintf("u1: %v", u1))
			} else if !ok2 {
				c.Echo("Finished")
				return
			}
		case u2, ok2 = <-upload2:
			if ok2 {
				c.Echo(fmt.Sprintf("u2: %v", u2))
			} else if !ok1 {
				c.Echo("Finished")
				return
			}
		}
	}
}

func (c *IACMessageBus) Abort() {
	fmt.Println("Abort")
	c.Hub.Abort()
}
*/
