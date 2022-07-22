package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/wmolicki/go-chat/pkg/message"
	"google.golang.org/protobuf/proto"
)

const ListenAddr = "localhost:8081"
const ClientsInfoPeriod = 5 * time.Second

var upgrader = websocket.Upgrader{}

var clients map[uuid.UUID]*client
var mu = sync.Mutex{}

var clientCount int

type client struct {
	clientId uuid.UUID
	conn     *websocket.Conn
	name     string
}

func (c client) String() string {
	return fmt.Sprintf("Client[%s (%s)]", c.name, c.clientId)
}

func getClientById(id uuid.UUID, clients map[uuid.UUID]*client) (*client, error) {
	for _, c := range clients {
		if c.clientId == id {
			return c, nil
		}
	}
	return nil, fmt.Errorf("no such client: %s", id)
}

func dummy(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatalf("could not upgrade conn to websocket: %v", err)
	}
	mu.Lock()
	clientCount += 1

	id, err := uuid.NewRandom()
	if err != nil {
		log.Fatalf("could not generate uuid: %v\n", err)
	}
	c := client{
		clientId: id,
		conn:     conn,
	}
	clients[c.clientId] = &c
	mu.Unlock()
	defer c.conn.Close()

	// send connected clients message immediately on connect so client
	// dont have to wait
	tempMap := map[uuid.UUID]*client{c.clientId: &c}
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
		// decoded, err := message.Decode(msg)
		// if err != nil {
		// 	log.Printf("skipping message - could not decode: %v\n", err)
		// }

		dest := &message.Message{}
		if err := proto.Unmarshal(msg, dest); err != nil {
			panic(err)
		}

		switch m := dest.Body.(type) {
		case *message.Message_ClientInfoMessage:
			log.Printf("got client info message from client %s: %s\n", c.clientId, m.ClientInfoMessage.Name)
			c.name = m.ClientInfoMessage.Name
		case *message.Message_ChatMessage:
			recipientId, err := uuid.FromBytes([]byte(m.ChatMessage.RecipientId))
			senderId, err := uuid.FromBytes([]byte(m.ChatMessage.SenderId))

			mu.Lock()
			recipient, err = getClientById(recipientId, clients)
			sender, err := getClientById(senderId, clients)
			mu.Unlock()
			if err != nil {
				log.Printf("client %s already disconnected: %v", recipientId, err)
				break
			}
			log.Printf("got chat message %+v from %s to %s\n", m, senderId, recipientId)

			prepared, err := websocket.NewPreparedMessage(1, msg)
			if err != nil {
				log.Fatalf("could not prepare message: %v", err)
			}

			err = recipient.conn.WritePreparedMessage(prepared)
			if err != nil {
				log.Printf("error writing message to client %d: %v\n", recipient.clientId, err)
				continue
			}
			err = sender.conn.WritePreparedMessage(prepared)
			if err != nil {
				log.Printf("error writing message to client %d: %v\n", sender.clientId, err)
				continue
			}
		default:
			log.Printf("got some weird message from client: %v", m)
		}

	}
}

func sendConnectedClientsMessage(clients map[uuid.UUID]*client, sendTo map[uuid.UUID]*client) {
	mu.Lock()
	defer mu.Unlock()
	connectedClients := &message.ConnectedClientsMessage{}

	ccs := []*message.ConnectedClientsMessage_ConnectedClient{}
	for id, c := range clients {
		if c.name == "" {
			continue
		}
		ccs = append(ccs, &message.ConnectedClientsMessage_ConnectedClient{Name: c.name, Id: id.String()})
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
	clients = make(map[uuid.UUID]*client)

	// send out information about the clients to every client
	go func() {
		t := time.NewTicker(ClientsInfoPeriod)
		defer t.Stop()
		for range t.C {
			sendConnectedClientsMessage(clients, clients)
		}
	}()

	http.HandleFunc("/", dummy)
	log.Println("started server")
	log.Fatal(http.ListenAndServe(ListenAddr, nil))
}
