package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wmolicki/go-chat/pkg/message"
)

const ListenAddr = "localhost:8081"
const ClientsInfoPeriod = 5 * time.Second

var upgrader = websocket.Upgrader{}

var clients map[int]*client
var mu = sync.Mutex{}

var clientCount int

type client struct {
	clientId int
	conn     *websocket.Conn
	name     string
}

func (c client) String() string {
	return fmt.Sprintf("Client[%s (%d)]", c.name, c.clientId)
}

func getClientByName(name string, clients map[int]*client) (*client, error) {
	for _, c := range clients {
		if c.name == name {
			return c, nil
		}
	}
	return nil, fmt.Errorf("no such client: %s", name)
}

func dummy(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatalf("could not upgrade conn to websocket: %v", err)
	}
	mu.Lock()
	clientCount += 1

	c := client{
		clientId: clientCount,
		conn:     conn,
	}
	clients[c.clientId] = &c
	mu.Unlock()
	defer c.conn.Close()

	// send connected clients message immediately on connect so client
	// dont have to wait
	tempMap := map[int]*client{c.clientId: &c}
	sendConnectedClientsMessage(clients, tempMap)

	var recipient *client

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("client %s disconnecting: %v\n", c, err)
			mu.Lock()
			delete(clients, c.clientId)
			mu.Unlock()
			break
		}
		decoded, err := message.Decode(msg)
		if err != nil {
			log.Printf("skipping message - could not decode: %v\n", err)
		}
		switch m := decoded.(type) {
		case *message.ClientInfoMessage:
			c.name = m.Name
			mu.Lock()
			recipient, err = getClientByName(m.Name, clients)
			mu.Unlock()
			if err != nil {
				log.Println("could not get client by name")
				continue
			}
			log.Printf("got client info message from client %s: %s\n", c, m.Name)
		case *message.ChatMessage:
			mu.Lock()
			recipient, err = getClientByName(m.Recipient, clients)
			mu.Unlock()
			if err != nil {
				log.Printf("client %s already disconnected", m.Recipient)
				break
			}
			log.Printf("got chat message %+v from %s to %s: %s\n", m, m.Sender, m.Recipient, m.Text)
		default:
			log.Printf("got some weird message from client: %v", m)
		}

		prepared, err := websocket.NewPreparedMessage(1, msg)
		if err != nil {
			log.Fatalf("could not prepare message: %v", err)
		}

		err = recipient.conn.WritePreparedMessage(prepared)
		if err != nil {
			log.Printf("error writing message to client %d: %v\n", recipient.clientId, err)
			continue
		}
	}
}

func sendConnectedClientsMessage(clients map[int]*client, sendTo map[int]*client) {
	mu.Lock()
	defer mu.Unlock()
	connectedClients := []string{}
	for _, c := range clients {
		if c.name == "" {
			continue
		}
		connectedClients = append(connectedClients, c.name)
	}
	m := message.ConnectedClientsMessage{Clients: connectedClients}
	encoded, err := m.Encode()
	if err != nil {
		log.Fatalf("could not encode connected clients: %v ", err)
	}

	prepared, err := websocket.NewPreparedMessage(1, encoded)
	if err != nil {
		log.Fatalf("error preparing message: %v\n", err)
	}

	for _, cl := range sendTo {
		err := cl.conn.WritePreparedMessage(prepared)
		if err != nil {
			log.Printf("error writing message to client %s: %v\n", cl, err)
			continue
		}
	}
}

func main() {
	clients = make(map[int]*client)

	// send out information about the clients to every client
	go func() {
		t := time.NewTicker(ClientsInfoPeriod)
		defer t.Stop()
		for range t.C {
			sendConnectedClientsMessage(clients, clients)
		}
	}()

	http.HandleFunc("/", dummy)
	log.Fatal(http.ListenAndServe(ListenAddr, nil))
}
