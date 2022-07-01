package main

import (
	"fmt"
	"log"
	"net/url"
	"sort"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/gorilla/websocket"
	"github.com/rivo/tview"
	"github.com/wmolicki/go-chat/pkg/message"
)

const ClientName = "wojtek"
const ServerURL = "localhost:8081"

var app *tview.Application

var connectedClients []string

type ChatEntry struct {
	Text string
	Time time.Time
}

func (e *ChatEntry) Format(username string) string {
	return fmt.Sprintf("%s %s %s", e.Time.Format("15:04"), username, e.Text)
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

	// var mu sync.Mutex

	connectedClients = []string{}
	chatHistory := make(map[string][]ChatEntry)

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

	app = tview.NewApplication()

	connected := tview.NewList()
	connected.SetBorder(true).SetTitle("Connected [CTRL+U]")
	connected.ShowSecondaryText(false)

	connected.SetFocusFunc(func() {
		connected.SetBorderColor(tcell.ColorDarkGreen)
	})
	connected.SetBlurFunc(func() {
		connected.SetBorderColor(tcell.ColorWhite)
	})

	chat := tview.NewList()
	chat.SetBorder(true).SetTitle("Chatting with <name>")

	input := tview.NewInputField()
	input.SetText("aaa")
	input.SetBorder(true).SetTitle("Message [CTRL+M]")
	input.SetFieldStyle(tcell.StyleDefault.Background(tview.Styles.PrimitiveBackgroundColor))

	input.SetFocusFunc(func() {
		input.SetBorderColor(tcell.ColorDarkGreen)
	})
	input.SetBlurFunc(func() {
		input.SetBorderColor(tcell.ColorWhite)
	})

	input.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			if input.GetText() == "" {
				break
			}
			sendCh <- []byte(input.GetText())
			input.SetText("")
		}
	})

	flex := tview.NewFlex().
		AddItem(connected, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(chat, 0, 1, false).
			AddItem(input, 3, 1, false), 0, 4, false)

	connected.SetSelectedTextColor(tcell.ColorDarkGreen)
	connected.SetHighlightFullLine(true)
	connected.SetChangedFunc(func(i int, username string, lel string, s rune) {
		chat.SetTitle(fmt.Sprintf("Chatting with %s", username))
		chat.Clear()
		entries, ok := chatHistory[username]
		if !ok {
			return
		}
		for _, e := range entries {
			chat.AddItem(e.Format(username), "", 0, func() {})
		}

	})
	connected.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		switch e.Key() {
		case tcell.KeyEnter:
			if connected.HasFocus() {
				app.SetFocus(input)
				return nil
			}
		}
		return e
	})

	app.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		switch e.Key() {
		case tcell.KeyEsc:
			app.Stop()
			return nil
		case tcell.KeyCtrlU:
			go app.QueueUpdateDraw(func() {
				app.SetFocus(connected)
			})
		case tcell.KeyCtrlM:
			go app.QueueUpdateDraw(func() {
				app.SetFocus(input)
			})
		case tcell.KeyRune:
			switch e.Rune() {
			case 'j':
				if connected.HasFocus() {
					connected.SetCurrentItem((connected.GetCurrentItem() + 1) % connected.GetItemCount())
				}
			case 'k':

				if connected.HasFocus() {
					connected.SetCurrentItem((connected.GetCurrentItem() - 1) % connected.GetItemCount())
				}
			}
		}
		return e
	})

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
				app.QueueUpdateDraw(func() {
					e := ChatEntry{Text: m.Text, Time: time.Now()}
					chatHistory[m.Username] = append(chatHistory[m.Username], e)
					chat.AddItem(e.Format(m.Username), "", 0, func() {})
				})
			case *message.ConnectedClientsMessage:
				app.QueueUpdateDraw(func() {
					connected.Clear()
					sort.Slice(m.Clients, func(i, j int) bool {
						return m.Clients[i] < m.Clients[j]
					})
					for _, c := range m.Clients {
						connected.AddItem(c, "", 0, nil)
					}
				})
			}
			if err != nil {
				log.Printf("could not decode message: %v, skipping", err)
				continue
			}
		}
	}()

	if err := app.SetRoot(flex, true).SetFocus(connected).Run(); err != nil {
		panic(err)
	}
}
