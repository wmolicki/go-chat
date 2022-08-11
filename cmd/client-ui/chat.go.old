package main

import (
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/wmolicki/go-chat/pkg/message"
	"github.com/wmolicki/go-datastructures/set"
)

type History struct {
	list   []*ChatEntry
	unread int
}

type Chat struct {
	// User
	Name string
	Id   uuid.UUID

	chatHistoryMu sync.Mutex
	chatHistory   map[uuid.UUID]*History

	ConnectedUsers []uuid.UUID

	currentRecipientMu sync.Mutex
	currentRecipientId *uuid.UUID

	conn *websocket.Conn

	recvCh chan []byte
	sendCh chan []byte

	connectedClientsMu sync.Mutex
	connectedClients   map[uuid.UUID]*ConnectedClient

	tui *TUI
}

func NewChat(username string) *Chat {
	c := Chat{Name: username, chatHistory: make(map[uuid.UUID]*History)}
	c.recvCh = make(chan []byte, 10)
	c.sendCh = make(chan []byte, 10)
	c.connectedClients = make(map[uuid.UUID]*ConnectedClient)
	return &c
}

func (c *Chat) GetHistoryFor(id uuid.UUID) []*ChatEntry {
	c.chatHistoryMu.Lock()
	defer c.chatHistoryMu.Unlock()
	connectedClient, err := c.FindById(id)
	if err != nil {
		log.Printf("error getting history: %s", err)
		return nil
	}
	log.Printf("getting history for %s, clearing unread\n", connectedClient.name)

	h, ok := c.chatHistory[id]
	if !ok {
		log.Printf("no history for %s\n", connectedClient.name)
		return []*ChatEntry{}
	}
	h.unread = 0
	return h.list
}

func (c *Chat) AppendHistory(senderId, recipientId uuid.UUID, message string) ChatEntry {
	sender, err := c.FindById(senderId)
	if err != nil {
		log.Printf("could not find sender: %s", err)
		return ChatEntry{}
	}
	recipient, err := c.FindById(recipientId)
	if err != nil {
		log.Printf("could not find recipient: %s", err)
		return ChatEntry{}
	}
	e := ChatEntry{Sender: sender.name, Recipient: recipient.name, Time: time.Now(), Text: message}

	c.chatHistoryMu.Lock()
	defer c.chatHistoryMu.Unlock()

	var target uuid.UUID
	if sender.name == c.Name {
		// messages sent by me are saved in chat history with recepient
		target = recipient.id
	} else {
		// messages sent to me are saved in chat history of the sender
		target = sender.id
	}
	log.Printf("saving message sent by %s to %s to %s[%s] = %s\n", sender.name, recipient.name, c.Name, target, message)
	h := c.chatHistory[target]
	if h == nil {
		h = &History{}
		c.chatHistory[target] = h
	}

	h.list = append(h.list, &e)
	return e
}

func (c *Chat) Send(message string) {
	c.sendCh <- []byte(message)
}

func (c *Chat) Find(displayName string) (*ConnectedClient, error) {
	for _, v := range c.connectedClients {
		if v.DisplayName() == displayName {
			return v, nil
		}
	}
	return nil, fmt.Errorf("%s is not connected", displayName)
}

func (c *Chat) FindById(id uuid.UUID) (*ConnectedClient, error) {
	v, ok := c.connectedClients[id]
	if !ok {
		return nil, fmt.Errorf("%s not connected", id)
	}
	return v, nil
}

func (c *Chat) Recv() ([]byte, bool) {
	msg, ok := <-c.recvCh
	return msg, ok
}

func (c *Chat) GetConnectedClients() map[uuid.UUID]*ConnectedClient {
	c.connectedClientsMu.Lock()
	defer c.connectedClientsMu.Unlock()
	return c.connectedClients
}

func (c *Chat) SetConnectedClients(connectedClients []message.ConnectedClient) {
	c.connectedClientsMu.Lock()
	defer c.connectedClientsMu.Unlock()
	clientsSet := set.NewSet[uuid.UUID]()
	for _, cc := range connectedClients {
		clientsSet.Add(cc.Id)
		_, ok := c.connectedClients[cc.Id]
		if !ok {
			c.connectedClients[cc.Id] = &ConnectedClient{id: cc.Id, name: cc.Name}
		}
	}
	// need to remove elements not present in connectedClients
	for k := range c.connectedClients {
		if !clientsSet.In(k) {
			delete(c.connectedClients, k)
		}
	}
}

func (c *Chat) Start() error {
	err := c.connectToChatServer()
	if err != nil {
		return fmt.Errorf("error connecting to chat server: %v", err)
	}
	err = c.sendClientInfo()
	if err != nil {
		return fmt.Errorf("error sending client info: %v", err)
	}

	// sending to server
	go func() {
		for {
			msg, ok := <-c.sendCh
			if !ok {
				log.Fatal("send channel closed - unhandled")
			}
			m := message.ChatMessage{SenderId: c.Id, Text: string(msg), RecipientId: *c.GetCurrentRecipientId()}
			log.Printf("Sending message (from %s) to %s: %s", m.SenderId, m.RecipientId, m.Text)
			encoded, err := m.Encode()
			if err != nil {
				log.Fatalf("could not encode message: %v", err)
			}
			err = c.conn.WriteMessage(1, encoded)
			if err != nil {
				log.Fatalf("could not send message via conn: %v", err)
			}
		}
	}()

	// receives messages from the server
	go func() {
		for {
			_, message, err := c.conn.ReadMessage()
			if err != nil {
				log.Printf("error reading message from conn: %v", err)
				close(c.recvCh)
				return
			}
			c.recvCh <- message
		}
	}()

	return nil
}

func (c *Chat) connectToChatServer() error {
	u := url.URL{Scheme: "ws", Host: ServerURL, Path: "/"}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("could not dial to ws server: %v", err)
	}
	c.conn = conn
	return nil
}

func (c *Chat) sendClientInfo() error {
	m := message.ClientInfoMessage{Name: c.Name}
	encoded, err := m.Encode()
	if err != nil {
		log.Fatalf("could not encode client info message: %v", err)
	}
	err = c.conn.WriteMessage(1, encoded)
	if err != nil {
		log.Fatalf("could not send message via conn: %v", err)
	}
	return nil
}

func (c *Chat) Close() error {
	return c.conn.Close()
}

func (c *Chat) SetCurrentRecipientId(id uuid.UUID) {
	c.currentRecipientMu.Lock()
	defer c.currentRecipientMu.Unlock()
	c.currentRecipientId = &id
}

func (c *Chat) GetCurrentRecipientId() *uuid.UUID {
	c.currentRecipientMu.Lock()
	defer c.currentRecipientMu.Unlock()
	return c.currentRecipientId
}

type ConnectedClient struct {
	unreadMu sync.Mutex
	unread   int

	id   uuid.UUID
	name string
}

func (c *ConnectedClient) DisplayName() string {
	if c.unread > 0 {
		return fmt.Sprintf("%s (%d)", c.name, c.unread)
	}
	return c.name
}

func (c *ConnectedClient) GetUnread() int {
	c.unreadMu.Lock()
	defer c.unreadMu.Unlock()

	return c.unread
}

func (c *ConnectedClient) IncUnread() int {
	c.unreadMu.Lock()
	defer c.unreadMu.Unlock()
	c.unread += 1
	return c.unread
}

func (c *ConnectedClient) ZeroUnread() {
	c.unreadMu.Lock()
	defer c.unreadMu.Unlock()
	c.unread = 0
}

type ChatEntry struct {
	Sender    string
	Recipient string
	Text      string
	Time      time.Time
}

func (e *ChatEntry) Format() string {
	return fmt.Sprintf("%s %s: %s", e.Time.Format("15:04"), e.Sender, e.Text)
}
