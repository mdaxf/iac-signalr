// Copyright 2023 IAC. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
/*
 the method is used by the signalr js client to call the method on the server
*/
package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/mdaxf/iac-signalr/logger"
	"github.com/mdaxf/iac-signalr/signalr"
)

type IACMessageBus struct {
	signalr.Hub
	ilog logger.Log
}

var groupname = "IAC_Internal_MessageBus"

func (c *IACMessageBus) Subscribe(topic string, connectionID string) {
	//fmt.Printf("Subscribe: topic: %s, sender: %s\n", topic, connectionID)
	c.ilog.Debug(fmt.Sprintf("Subscribe: topic: %s, sender: %s\n", topic, connectionID))
}
func (c *IACMessageBus) Send(topic string, message string, connectionID string) {
	c.ilog.Debug(fmt.Sprintf("Send: topic: %s, message: %s, sender: %s\n", topic, message, connectionID))
	c.Clients().Group(groupname).Send(topic, message)
	//	c.Clients().Caller().Send("receive", message)
}

func (c *IACMessageBus) SendToBackEnd(topic string, message string, connectionID string) {
	c.ilog.Debug(fmt.Sprintf("SendToBackEnd: topic: %s, message: %s, sender: %s\n", topic, message, connectionID))
	JsonMsg := make(map[string]interface{}) //"{\"topic\":\"" + topic + "\",\"message\":\"" + message + "\",\"sender\":\"" + connectionID + "\"}"
	JsonMsg["topic"] = topic
	JsonMsg["message"] = message
	JsonMsg["sender"] = connectionID
	//	fmt.Printf("SendToBackEnd: JsonMsg: %s\n", JsonMsg)

	c.ilog.Debug(fmt.Sprintf("SendToBackEnd: JsonMsg: %s\n", JsonMsg))
	c.Clients().Group(groupname).Send("sendtobackend", JsonMsg)
	//	c.Clients().Caller().Send("receive", message)
}

func (c *IACMessageBus) AddMessage(message string, topic string, sender string) {
	ilog.Debug(fmt.Sprintf("AddMessage: topic: %s, message: %s, sender: %s\n", topic, message, sender))
	c.Clients().Group(groupname).Send(topic, message)
}

// add the client to the connection
func (c *IACMessageBus) OnConnected(connectionID string) {
	c.ilog.Debug(fmt.Sprintf("%s connected\n", connectionID))
	c.Groups().AddToGroup(groupname, connectionID)
	fmt.Printf("%s connected and added to group %s\n", connectionID, groupname)
}

func (c *IACMessageBus) OnDisconnected(connectionID string) {
	c.ilog.Debug(fmt.Sprintf("%s disconnected\n", connectionID))
	c.Groups().RemoveFromGroup(groupname, connectionID)
	ilog.Debug(fmt.Sprintf("%s disconnected and removed from group %s\n", connectionID, groupname))
}

func (c *IACMessageBus) Broadcast(message string) {
	// Broadcast to all clients
	c.ilog.Debug(fmt.Sprintf("broadcast message: %s\n", message))
	c.Clients().Group(groupname).Send("broadcast", message)
	c.Clients().Group(groupname).Send("receive", message)
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
