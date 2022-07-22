package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/gdamore/tcell/v2"
	"github.com/google/uuid"

	"github.com/wmolicki/go-chat/pkg/message"
)

const ServerURL = "localhost:8081"

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

func handleChatMessage(chat *Chat, tui *TUI, m *message.ChatMessage) error {
	e := chat.AppendHistory(m.SenderId, m.RecipientId, m.Text)
	if *chat.GetCurrentRecipientId() == m.SenderId || *chat.GetCurrentRecipientId() == m.RecipientId {
		tui.app.QueueUpdateDraw(func() {
			tui.history.AddItem(e.Format(), "", 0, func() {})
		})
	} else {
		// new message in hidden chat, add the "new message" indicator
		tui.app.QueueUpdateDraw(func() {
			var targetId uuid.UUID
			if m.SenderId == chat.Id {
				// messages sent by me are saved in chat history with recepient
				targetId = m.RecipientId
			} else {
				// messages sent to me are saved in chat history of the sender
				targetId = m.SenderId
			}
			targetClient, err := chat.FindById(targetId)
			if err != nil {
				log.Printf("hidden message for unknown client: %s: %s\n", targetId, err)
			}
			targetClient.IncUnread()
			tui.updateRecipientNameInChat(chat, &targetId)
		})
	}
	return nil
}

func handleConnectedClientsMessage(chat *Chat, tui *TUI, m *message.ConnectedClientsMessage) error {
	tui.app.QueueUpdateDraw(func() {
		sort.Slice(m.Clients, func(i, j int) bool {
			return m.Clients[i].Name < m.Clients[j].Name
		})

		// not a copy
		currentlyConnected := chat.GetConnectedClients()
		currentlyConnectedCopy := make(map[uuid.UUID]struct{})
		for k, _ := range currentlyConnected {
			currentlyConnectedCopy[k] = struct{}{}
		}
		serverConnected := make(map[uuid.UUID]struct{})

		chat.SetConnectedClients(m.Clients)

		// new connected cliens
		for _, c := range m.Clients {
			// dont display my own username in chat
			if _, ok := currentlyConnectedCopy[c.Id]; !ok && c.Name != chat.Name {
				// not in currently connected, need to add
				tui.connected.AddItem(c.Name, c.Id.String(), 0, nil)
			} else if c.Name == chat.Name {
				chat.Id = c.Id
			}
			serverConnected[c.Id] = struct{}{}
		}
		// deleting no longer connected
		for k, _ := range currentlyConnectedCopy {
			if _, ok := serverConnected[k]; !ok {
				// not in server connected, need to delete
				// (need to find by display name of client)
				items := tui.connected.FindItems("", k.String(), true, false)
				if len(items) > 0 {
					log.Printf("removing %v\n", items)
					tui.connected.RemoveItem(items[0])
				}
			}
		}

		// first-time population of currentRecepientId when there is at
		// least one client connected
		if chat.currentRecipientId == nil && tui.connected.GetItemCount() > 0 {
			username, _ := tui.connected.GetItemText(tui.connected.GetCurrentItem())
			tui.setCurrentRecipient(chat, username)
		}
	})
	return nil
}

func main() {
	config := parseFlags()

	logFile, err := os.OpenFile(fmt.Sprintf("%s_chat.log", config.UserName), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal("could not open log file")
	}
	defer logFile.Close()
	log.SetOutput(logFile)
	chat := NewChat(config.UserName)
	err = chat.Start()
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

	// this fires when we change recepient in the list
	// (also fires when its the first time)
	tui.connected.SetChangedFunc(func(i int, username string, lel string, s rune) {
		tui.setCurrentRecipient(chat, username)
		tui.updateRecipientNameInChat(chat, chat.GetCurrentRecipientId())
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
				log.Printf("Received message from %s to %s: %s", m.SenderId, m.RecipientId, m.Text)
				err := handleChatMessage(chat, tui, m)
				if err != nil {
					fmt.Printf("error handling message: %s\n", err)
				}
			case *message.ConnectedClientsMessage:
				err := handleConnectedClientsMessage(chat, tui, m)
				if err != nil {
					fmt.Printf("error handling message: %s\n", err)
				}
			}
			if err != nil {
				log.Printf("could not decode message: %v, skipping", err)
				continue
			}
		}
	}()

	log.Println("starting..")
	if err := tui.Run(); err != nil {
		panic(err)
	}
}
