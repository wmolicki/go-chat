package main

import (
	"bufio"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/wmolicki/go-chat/pkg/message"
)

const ClientName = "kasia"
const Prompt = ">>> "
const ServerURL = "localhost:8081"

func connectToChatServer() (*websocket.Conn, error) {
	u := url.URL{Scheme: "ws", Host: ServerURL, Path: "/"}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("could not dial to ws server: %v", err)
	}

	return conn, nil
}

func sendClientInfo(conn *websocket.Conn) error {
	m := message.ClientInfoMessage{Name: ClientName}
	encoded, err := m.Encode()
	if err != nil {
		log.Fatalf("could not encode client info message: %v", err)
	}
	err = conn.WriteMessage(1, encoded)
	if err != nil {
		log.Fatalf("could not send message via conn: %v", err)
	}
	return nil
}

func main() {
	conn, err := connectToChatServer()
	if err != nil {
		log.Fatalf("error connecting to chat server: %v", err)
	}
	defer conn.Close()
	err = sendClientInfo(conn)
	if err != nil {
		log.Fatalf("error sending client info: %v", err)
	}

	recvCh := make(chan []byte, 10)
	sendCh := make(chan []byte, 10)

	// sending to server
	go func() {
		for {
			msg, ok := <-sendCh
			if !ok {
				log.Fatal("send channel closed - unhandled")
			}
			m := message.NewChatMessage(ClientName, string(msg))
			encoded, err := m.Encode()
			if err != nil {
				log.Fatalf("could not encode message: %v", err)
			}
			err = conn.WriteMessage(1, encoded)
			if err != nil {
				log.Fatalf("could not send message via conn: %v", err)
			}
		}
	}()

	// receives messages from the server
	go func() {
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("error reading message from conn: %v", err)
				close(recvCh)
				return
			}
			recvCh <- message
		}
	}()

	// decodes and prints messages from the receive queue
	go func() {
		for {
			msg, ok := <-recvCh
			if !ok {
				log.Fatalf("message channel closed - this is not intended atm")
			}
			decoded, err := message.Decode(msg)
			switch m := decoded.(type) {
			case message.ChatMessage:
				fmt.Printf("%s: %s\n"+Prompt, m.Username, m.Text)
			default:
				fmt.Printf("other message: %v\n", m)
			}
			if err != nil {
				log.Printf("could not decode message: %v, skipping", err)
				continue
			}
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print(Prompt)
scan:
	for scanner.Scan() {
		message := strings.TrimSpace(scanner.Text())
		switch message {
		case "\\quit", "\\q":
			break scan
		default:
			sendCh <- []byte(message)
		}
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}
}
