package main

import (
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/gorilla/websocket"
	"github.com/marcusolsson/tui-go"
	"github.com/wmolicki/go-chat/pkg/message"
)

const ClientName = "wojtek"
const Prompt = ">>> "
const ServerURL = "localhost:8081"

var connectedClients []string

type layout struct {
	sidebar *tui.Box
	history *tui.Box
	input   *tui.Entry
	ui      tui.UI
}

func setupUI(sendCh chan []byte) *layout {
	// should be updated from the server
	sidebar := tui.NewVBox(
		tui.NewLabel("PEOPLE"),
	)

	history := tui.NewVBox()
	historyScroll := tui.NewScrollArea(history)
	historyScroll.SetAutoscrollToBottom(true)

	historyBox := tui.NewVBox(historyScroll)
	historyBox.SetBorder(true)

	input := tui.NewEntry()
	input.SetFocused(true)
	input.SetSizePolicy(tui.Expanding, tui.Maximum)

	inputBox := tui.NewHBox(input)
	inputBox.SetBorder(true)
	inputBox.SetSizePolicy(tui.Expanding, tui.Maximum)

	chat := tui.NewVBox(historyBox, inputBox)
	chat.SetSizePolicy(tui.Expanding, tui.Expanding)

	// should only send to sending queue
	input.OnSubmit(func(e *tui.Entry) {
		sendCh <- []byte(e.Text())
		input.SetText("")
	})

	// history should be updated from the server

	root := tui.NewHBox(sidebar, chat)

	ui, err := tui.New(root)
	if err != nil {
		log.Fatal(err)
	}

	ui.SetKeybinding("Esc", func() { ui.Quit() })

	return &layout{
		sidebar: sidebar,
		history: history,
		input:   input,
		ui:      ui,
	}
}

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

	var mu sync.Mutex
	connectedClients = []string{}

	l := setupUI(sendCh)

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
			case *message.ChatMessage:
				l.history.Append(tui.NewHBox(
					tui.NewLabel(time.Now().Format("15:04")),
					tui.NewPadder(1, 0, tui.NewLabel(m.Username)),
					tui.NewLabel(m.Text),
					tui.NewSpacer(),
				))
				l.ui.Repaint()
			case *message.ConnectedClientsMessage:
				less := func(a, b string) bool { return a < b }
				mu.Lock()
				equalIgnoreOrder := cmp.Equal(connectedClients, m.Clients, cmpopts.SortSlices(less))
				if equalIgnoreOrder == true {
					mu.Unlock()
					break
				}
				connectedClients = append(connectedClients)
				for l.sidebar.Length() > 1 {
					l.sidebar.Remove(1)
				}
				for _, cn := range m.Clients {
					l.sidebar.Append(tui.NewLabel(cn))
				}
				l.sidebar.Append(tui.NewSpacer())
				l.ui.Repaint()
			}
			if err != nil {
				log.Printf("could not decode message: %v, skipping", err)
				continue
			}
		}
	}()

	l.ui.Update
	if err := l.ui.Run(); err != nil {
		log.Fatal(err)
	}
}
