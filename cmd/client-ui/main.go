package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/gorilla/websocket"
	"github.com/rivo/tview"
	"github.com/wmolicki/go-chat/pkg/message"
)

const ServerURL = "localhost:8081"

type User struct {
	Name string

	chatHistoryMu sync.Mutex
	chatHistory   map[string][]ChatEntry
}

func (u *User) GetHistoryFor(username string) []ChatEntry {
	u.chatHistoryMu.Lock()
	defer u.chatHistoryMu.Unlock()

	entries, ok := u.chatHistory[username]
	if !ok {
		return []ChatEntry{}
	}
	return entries
}

func (u *User) AppendHistory(sender string, recipient string, message string) ChatEntry {
	e := ChatEntry{Sender: sender, Recipient: recipient, Time: time.Now()}
	u.chatHistoryMu.Lock()
	defer u.chatHistoryMu.Unlock()
	var target string
	if sender == u.Name {
		// messages sent by me are saved in chat history with recepient
		target = recipient
	} else {
		// messages sent to me are saved in chat history of the sender
		target = sender
	}
	u.chatHistory[target] = append(u.chatHistory[target], e)
	return e
}

func NewUser(name string) *User {
	u := User{Name: name, chatHistory: make(map[string][]ChatEntry)}
	return &u
}

type Chat struct {
	ConnectedUsers []string

	currentRecipientMu sync.Mutex
	currentRecipient   string

	conn *websocket.Conn

	recvCh chan []byte
	sendCh chan []byte

	connectedClientsMu sync.Mutex
	connectedClients   []string
}

func (c *Chat) Send(message string) {
	c.sendCh <- []byte(message)
}

func (c *Chat) Recv() ([]byte, bool) {
	msg, ok := <-c.recvCh
	return msg, ok
}

func NewChat() *Chat {
	c := Chat{}
	c.recvCh = make(chan []byte, 10)
	c.sendCh = make(chan []byte, 10)
	return &c
}

func (c *Chat) GetConnectedClients() []string {
	c.connectedClientsMu.Lock()
	defer c.connectedClientsMu.Unlock()
	return c.connectedClients
}

func (c *Chat) SetConnectedClients(clients []string) {
	c.connectedClientsMu.Lock()
	defer c.connectedClientsMu.Unlock()
	c.connectedClients = c.connectedClients[:0]
	c.connectedClients = append(c.connectedClients, clients...)
	// sort.Slice(c.connectedClients, func(i, j int) bool {
	// 	return c.connectedClients[i] < c.connectedClients[j]
	// })
}

func (c *Chat) Start(user *User) error {
	err := c.connectToChatServer()
	if err != nil {
		return fmt.Errorf("error connecting to chat server: %v", err)
	}
	err = c.sendClientInfo(user)
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
			m := message.NewChatMessage(user.Name, string(msg), c.GetCurrentRecipient())
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

func (c *Chat) sendClientInfo(user *User) error {
	m := message.ClientInfoMessage{Name: user.Name}
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

func (c *Chat) SetCurrentRecipient(name string) {
	c.currentRecipientMu.Lock()
	defer c.currentRecipientMu.Unlock()
	c.currentRecipient = name
}

func (c *Chat) GetCurrentRecipient() string {
	c.currentRecipientMu.Lock()
	defer c.currentRecipientMu.Unlock()
	return c.currentRecipient
}

type ChatEntry struct {
	Sender    string
	Recipient string
	Text      string
	Time      time.Time
}

func (e *ChatEntry) Format() string {
	return fmt.Sprintf("%s %s %s", e.Time.Format("15:04"), e.Sender, e.Text)
}

type Config struct {
	UserName string
}

func parseFlags() Config {
	userNamePtr := flag.String("username", "", "chat username")
	flag.Parse()
	if *userNamePtr == "" {
		return Config{UserName: "debug"}
		// log.Fatal("username must be set")
	}

	c := Config{UserName: *userNamePtr}

	return c
}

type TUI struct {
	app       *tview.Application
	connected *tview.List
	history   *tview.List
	input     *tview.InputField

	flex *tview.Flex
}

func (t *TUI) Run() error {
	err := t.app.SetRoot(t.flex, true).SetFocus(t.connected).Run()
	return err
}

func NewTUI() *TUI {
	app := tview.NewApplication()

	connected := tview.NewList()
	connected.SetBorder(true).SetTitle("Connected [CTRL+U]")
	connected.ShowSecondaryText(false)

	connected.SetFocusFunc(func() {
		connected.SetBorderColor(tcell.ColorDarkGreen)
	})
	connected.SetBlurFunc(func() {
		connected.SetBorderColor(tcell.ColorWhite)
	})
	connected.SetSelectedTextColor(tcell.ColorDarkGreen)
	connected.SetHighlightFullLine(true)

	chat := tview.NewList()
	chat.SetBorder(true).SetTitle("Chatting with <name>")

	input := tview.NewInputField()
	input.SetBorder(true).SetTitle("Message [CTRL+M]")
	input.SetFieldStyle(tcell.StyleDefault.Background(tview.Styles.PrimitiveBackgroundColor))
	input.SetFocusFunc(func() {
		input.SetBorderColor(tcell.ColorDarkGreen)
	})
	input.SetBlurFunc(func() {
		input.SetBorderColor(tcell.ColorWhite)
	})

	flex := tview.NewFlex().
		AddItem(connected, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(chat, 0, 1, false).
			AddItem(input, 3, 1, false), 0, 4, false)

	return &TUI{
		app:       app,
		connected: connected,
		history:   chat,
		input:     input,
		flex:      flex,
	}
}

func main() {
	config := parseFlags()

	user := NewUser(config.UserName)
	chat := NewChat()
	err := chat.Start(user)
	if err != nil {
		log.Fatal(err)
	}
	defer chat.Close()

	tui := NewTUI()

	tui.input.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			if tui.input.GetText() == "" {
				break
			}
			chat.Send(tui.input.GetText())
			tui.input.SetText("")
		}
	})

	tui.connected.SetChangedFunc(func(i int, username string, lel string, s rune) {
		chat.SetCurrentRecipient(username)
		tui.history.SetTitle(fmt.Sprintf("(%s) Chatting with %s", user.Name, username))
		tui.history.Clear()
		entries := user.GetHistoryFor(username)
		for _, e := range entries {
			tui.history.AddItem(e.Format(), "", 0, func() {})
		}

	})
	tui.connected.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		switch e.Key() {
		case tcell.KeyEnter:
			if tui.connected.HasFocus() {
				tui.app.SetFocus(tui.input)
				return nil
			}
		}
		return e
	})

	tui.app.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		switch e.Key() {
		case tcell.KeyEsc:
			tui.app.Stop()
			return nil
		case tcell.KeyCtrlU:
			go tui.app.QueueUpdateDraw(func() {
				tui.app.SetFocus(tui.connected)
			})
		case tcell.KeyCtrlM:
			go tui.app.QueueUpdateDraw(func() {
				tui.app.SetFocus(tui.input)
			})
		case tcell.KeyRune:
			switch e.Rune() {
			case 'j':
				if tui.connected.HasFocus() {
					tui.connected.SetCurrentItem((tui.connected.GetCurrentItem() + 1) % tui.connected.GetItemCount())
				}
			case 'k':

				if tui.connected.HasFocus() {
					tui.connected.SetCurrentItem((tui.connected.GetCurrentItem() - 1) % tui.connected.GetItemCount())
				}
			}
		}
		return e
	})

	// decodes and prints messages from the receive queue
	go func() {
		for {
			msg, ok := chat.Recv()
			if !ok {
				log.Fatalf("message channel closed - this is not intended atm")
			}
			decoded, err := message.Decode(msg)
			switch m := decoded.(type) {
			case *message.ChatMessage:
				tui.app.QueueUpdateDraw(func() {
					e := user.AppendHistory(m.Sender, m.Recipient, m.Text)
					tui.history.AddItem(e.Format(), "", 0, func() {})
				})
			case *message.ConnectedClientsMessage:
				tui.app.QueueUpdateDraw(func() {
					sort.Slice(m.Clients, func(i, j int) bool {
						return m.Clients[i] < m.Clients[j]
					})
					if reflect.DeepEqual(m.Clients, chat.GetConnectedClients()) {
						return
					}
					tui.connected.Clear()
					chat.SetConnectedClients(m.Clients)
					for _, c := range m.Clients {
						if c != user.Name {
							tui.connected.AddItem(c, "", 0, nil)
						}
					}
				})
			}
			if err != nil {
				log.Printf("could not decode message: %v, skipping", err)
				continue
			}
		}
	}()

	if err := tui.Run(); err != nil {
		panic(err)
	}
}
