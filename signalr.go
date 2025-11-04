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
	"sync"
	"time"

	"github.com/mdaxf/iac-signalr/logger"
	"github.com/mdaxf/iac-signalr/signalr"
)

// ConnectionInfo tracks details about each connection
type ConnectionInfo struct {
	ID           string
	ConnectedAt  time.Time
	LastActivity time.Time
	Topics       []string
}

type IACMessageBus struct {
	signalr.Hub
	ilog             logger.Log
	connectionsMutex sync.RWMutex
	connections      map[string]*ConnectionInfo
	totalConnections uint64 // Total connections since server start
}

var groupname = "IAC_Internal_MessageBus"

const (
	MaxMessageSize = 1024 * 1024 // 1MB
	MaxTopicLength = 256
)

// validateTopic validates the topic string
func validateTopic(topic string) error {
	if len(topic) == 0 {
		return fmt.Errorf("topic cannot be empty")
	}
	if len(topic) > MaxTopicLength {
		return fmt.Errorf("topic exceeds maximum length of %d", MaxTopicLength)
	}
	// Check for invalid characters
	if strings.ContainsAny(topic, "\n\r\t") {
		return fmt.Errorf("topic contains invalid characters")
	}
	return nil
}

// validateMessage validates the message string
func validateMessage(message string) error {
	if len(message) > MaxMessageSize {
		return fmt.Errorf("message exceeds maximum size of %d bytes", MaxMessageSize)
	}
	return nil
}

func (c *IACMessageBus) Subscribe(topic string, connectionID string) {
	// Validate topic
	if err := validateTopic(topic); err != nil {
		c.ilog.Warn(fmt.Sprintf("Invalid topic in Subscribe - connectionID=%s error=%v", connectionID, err))
		c.Clients().Caller().Send("error", map[string]string{
			"code":    "INVALID_TOPIC",
			"message": "Topic validation failed",
		})
		return
	}

	c.ilog.Debug(fmt.Sprintf("Subscribe: topic=%s connectionID=%s", topic, connectionID))
}
func (c *IACMessageBus) Send(topic string, message string, connectionID string) {
	// Validate topic
	if err := validateTopic(topic); err != nil {
		c.ilog.Warn(fmt.Sprintf("Invalid topic in Send - connectionID=%s error=%v", connectionID, err))
		c.Clients().Caller().Send("error", map[string]string{
			"code":    "INVALID_TOPIC",
			"message": "Topic validation failed",
		})
		return
	}

	// Validate message
	if err := validateMessage(message); err != nil {
		c.ilog.Warn(fmt.Sprintf("Invalid message in Send - connectionID=%s error=%v", connectionID, err))
		c.Clients().Caller().Send("error", map[string]string{
			"code":    "INVALID_MESSAGE",
			"message": "Message validation failed",
		})
		return
	}

	c.ilog.Debug(fmt.Sprintf("Send - topic=%s messageSize=%d sender=%s", topic, len(message), connectionID))
	c.Clients().Group(groupname).Send(topic, message)
}

func (c *IACMessageBus) SendToBackEnd(topic string, message string, connectionID string) {
	// Validate topic
	if err := validateTopic(topic); err != nil {
		c.ilog.Warn(fmt.Sprintf("Invalid topic in SendToBackEnd - connectionID=%s error=%v", connectionID, err))
		c.Clients().Caller().Send("error", map[string]string{
			"code":    "INVALID_TOPIC",
			"message": "Topic validation failed",
		})
		return
	}

	// Validate message
	if err := validateMessage(message); err != nil {
		c.ilog.Warn(fmt.Sprintf("Invalid message in SendToBackEnd - connectionID=%s error=%v", connectionID, err))
		c.Clients().Caller().Send("error", map[string]string{
			"code":    "INVALID_MESSAGE",
			"message": "Message validation failed",
		})
		return
	}

	c.ilog.Debug(fmt.Sprintf("SendToBackEnd - topic=%s messageSize=%d sender=%s", topic, len(message), connectionID))

	JsonMsg := make(map[string]interface{})
	JsonMsg["topic"] = topic
	JsonMsg["message"] = message
	JsonMsg["sender"] = connectionID

	c.ilog.Debug(fmt.Sprintf("SendToBackEnd: JsonMsg=%v", JsonMsg))
	c.Clients().Group(groupname).Send("sendtobackend", JsonMsg)
}

func (c *IACMessageBus) AddMessage(message string, topic string, sender string) {
	// Validate topic
	if err := validateTopic(topic); err != nil {
		c.ilog.Warn(fmt.Sprintf("Invalid topic in AddMessage - sender=%s error=%v", sender, err))
		c.Clients().Caller().Send("error", map[string]string{
			"code":    "INVALID_TOPIC",
			"message": "Topic validation failed",
		})
		return
	}

	// Validate message
	if err := validateMessage(message); err != nil {
		c.ilog.Warn(fmt.Sprintf("Invalid message in AddMessage - sender=%s error=%v", sender, err))
		c.Clients().Caller().Send("error", map[string]string{
			"code":    "INVALID_MESSAGE",
			"message": "Message validation failed",
		})
		return
	}

	c.ilog.Debug(fmt.Sprintf("AddMessage - topic=%s messageSize=%d sender=%s", topic, len(message), sender))
	c.Clients().Group(groupname).Send(topic, message)
}

// add the client to the connection
func (c *IACMessageBus) OnConnected(connectionID string) {
	c.connectionsMutex.Lock()
	defer c.connectionsMutex.Unlock()

	// Initialize connections map if needed
	if c.connections == nil {
		c.connections = make(map[string]*ConnectionInfo)
	}

	// Track connection info
	connInfo := &ConnectionInfo{
		ID:           connectionID,
		ConnectedAt:  time.Now(),
		LastActivity: time.Now(),
		Topics:       []string{},
	}
	c.connections[connectionID] = connInfo
	c.totalConnections++

	c.Groups().AddToGroup(groupname, connectionID)

	c.ilog.Info(fmt.Sprintf("Client connected - connectionID=%s group=%s totalActive=%d totalConnections=%d",
		connectionID, groupname, len(c.connections), c.totalConnections))
}

func (c *IACMessageBus) OnDisconnected(connectionID string) {
	c.connectionsMutex.Lock()
	defer c.connectionsMutex.Unlock()

	connInfo, exists := c.connections[connectionID]
	if exists {
		duration := time.Since(connInfo.ConnectedAt)
		c.ilog.Info(fmt.Sprintf("Client disconnected - connectionID=%s duration=%v topics=%d totalActive=%d",
			connectionID, duration, len(connInfo.Topics), len(c.connections)-1))
		delete(c.connections, connectionID)
	} else {
		c.ilog.Debug(fmt.Sprintf("Client disconnected - connectionID=%s (not tracked)", connectionID))
	}

	c.Groups().RemoveFromGroup(groupname, connectionID)
}

func (c *IACMessageBus) Broadcast(message string) {
	// Validate message
	if err := validateMessage(message); err != nil {
		c.ilog.Warn(fmt.Sprintf("Invalid message in Broadcast - error=%v", err))
		c.Clients().Caller().Send("error", map[string]string{
			"code":    "INVALID_MESSAGE",
			"message": "Message validation failed",
		})
		return
	}

	c.ilog.Debug(fmt.Sprintf("Broadcast - messageSize=%d", len(message)))
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
	c.ilog.Warn("Connection abort requested")
	c.Hub.Abort()
}

// GetConnectionCount returns the current number of active connections
func (c *IACMessageBus) GetConnectionCount() int {
	c.connectionsMutex.RLock()
	defer c.connectionsMutex.RUnlock()
	return len(c.connections)
}

// GetTotalConnections returns the total number of connections since server start
func (c *IACMessageBus) GetTotalConnections() uint64 {
	c.connectionsMutex.RLock()
	defer c.connectionsMutex.RUnlock()
	return c.totalConnections
}

// GetConnectionInfo returns information about a specific connection
func (c *IACMessageBus) GetConnectionInfo(connectionID string) (*ConnectionInfo, bool) {
	c.connectionsMutex.RLock()
	defer c.connectionsMutex.RUnlock()
	info, exists := c.connections[connectionID]
	return info, exists
}
