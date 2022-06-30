package main

import (
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wmolicki/go-chat/pkg/message"
)

const ListenAddr = "localhost:8081"
const ClientsInfoPeriod = 5 * time.Second

var upgrader = websocket.Upgrader{}

var clients = map[int]*websocket.Conn{}
var mu = sync.Mutex{}

var clientCount int

func dummy(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatalf("could not upgrade conn to websocket: %v", err)
	}
	var clientId int
	mu.Lock()
	clientCount += 1
	clientId = clientCount
	clients[clientId] = conn
	mu.Unlock()
	defer conn.Close()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("client %d disconnecting: %v\n", clientId, err)
			mu.Lock()
			delete(clients, clientId)
			defer mu.Unlock()
			break
		}
		decoded, err := message.Decode(msg)
		if err != nil {
			log.Printf("skipping message - could not decode: %v\n", err)
		}
		switch m := decoded.(type) {
		case *message.ClientInfoMessage:
			log.Printf("got client info message from client %d: %s\n", clientId, m.Name)
		case *message.ChatMessage:
			log.Printf("got chat message from client %d: %s %s\n", clientId, m.Username, m.Text)
		default:
			log.Printf("got some weird message from client: %v", m)
		}

		prepared, err := websocket.NewPreparedMessage(1, msg)
		if err != nil {
			log.Fatalf("could not prepare message: %v", err)
		}
		mu.Lock()
		// this could be improved to send concurrently - but for now meh
		for k, conn := range clients {
			err = conn.WritePreparedMessage(prepared)
			if err != nil {
				log.Printf("error writing message to client %d: %v\n", k, err)
				continue
			}
		}
		mu.Unlock()
	}
}

func main() {
	clients = make(map[int]*websocket.Conn)

	// send out information about the clients to every client
	go func() {
		t := time.NewTicker(ClientsInfoPeriod)
		defer t.Stop()
		for range t.C {
			mu.Lock()
			connectedClients := []string{}
			for k := range clients {
				connectedClients = append(connectedClients, strconv.Itoa(k))
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

			for k, conn := range clients {
				err := conn.WritePreparedMessage(prepared)
				if err != nil {
					log.Printf("error writing message to client %d: %v\n", k, err)
					continue
				}
			}
			mu.Unlock()
		}
	}()

	http.HandleFunc("/", dummy)
	log.Fatal(http.ListenAndServe(ListenAddr, nil))
}
