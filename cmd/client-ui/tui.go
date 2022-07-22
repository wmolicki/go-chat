package main

import (
	"fmt"
	"log"

	"github.com/gdamore/tcell/v2"
	"github.com/google/uuid"
	"github.com/rivo/tview"
)

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

func (tui *TUI) updateRecipientNameInChat(chat *Chat, targetId *uuid.UUID) {
	// first time connecters wont have any unreads, so this is a yikes fix
	if targetId == nil {
		return
	}
	targetClient, err := chat.FindById(*targetId)
	if err != nil {
		log.Printf("could not find target for %s", targetId)
		return
	}
	t := tui.connected.FindItems("", targetId.String(), true, false)
	newName := targetClient.DisplayName()
	tui.connected.SetItemText(t[0], newName, targetId.String())
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

func (tui *TUI) setCurrentRecipient(chat *Chat, username string) {
	connClient, err := chat.Find(username)
	if err != nil {
		fmt.Errorf("connected error: %s", err)
		return
	}
	chat.SetCurrentRecipientId(connClient.id)
	log.Printf("connected changed to %s (%s)\n", username, chat.GetCurrentRecipientId())

	tui.history.SetTitle(fmt.Sprintf("(%s) Chatting with %s", chat.Name, connClient.name))
	tui.history.Clear()
	entries := chat.GetHistoryFor(connClient.id)
	for _, e := range entries {
		tui.history.AddItem(e.Format(), "", 0, func() {})
	}
	connClient.ZeroUnread()
}
