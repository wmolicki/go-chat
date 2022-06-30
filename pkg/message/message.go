package message

import (
	"encoding/json"
	"fmt"
	"log"
)

const (
	ChatMessageType int = iota
	ClientInfoMessageType
	ConnectedClientsMessageType
)

type Message struct {
	Type int
	Body json.RawMessage
}

type ChatMessage struct {
	Text     string `json:"text"`
	Username string `json:"username"`
}

type ClientInfoMessage struct {
	Name string `json:"name"`
}

func (c ClientInfoMessage) Encode() ([]byte, error) {
	bodyBytes, err := json.Marshal(c)
	if err != nil {
		return []byte{}, err
	}
	bodyJSONRaw := json.RawMessage(bodyBytes)
	e := Message{Type: ClientInfoMessageType, Body: bodyJSONRaw}
	messageBytes, err := json.Marshal(e)
	if err != nil {
		return []byte{}, err
	}

	return messageBytes, nil
}

type ConnectedClientsMessage struct {
	Clients []string `json:"clients"`
}

func (c ConnectedClientsMessage) Encode() ([]byte, error) {
	bodyBytes, err := json.Marshal(c)
	if err != nil {
		return []byte{}, err
	}
	bodyJSONRaw := json.RawMessage(bodyBytes)
	e := Message{Type: ConnectedClientsMessageType, Body: bodyJSONRaw}
	messageBytes, err := json.Marshal(e)
	if err != nil {
		return []byte{}, err
	}

	return messageBytes, nil
}

func NewChatMessage(username string, message string) ChatMessage {
	return ChatMessage{Username: username, Text: message}
}

func (c ChatMessage) Encode() ([]byte, error) {
	bodyBytes, err := json.Marshal(c)
	if err != nil {
		return []byte{}, err
	}
	bodyJSONRaw := json.RawMessage(bodyBytes)
	e := Message{Type: ChatMessageType, Body: bodyJSONRaw}
	messageBytes, err := json.Marshal(e)
	if err != nil {
		return []byte{}, err
	}

	return messageBytes, nil
}

func Decode(payload []byte) (interface{}, error) {
	m := Message{}
	err := json.Unmarshal(payload, &m)
	if err != nil {
		return m, fmt.Errorf("error unmarshaling payload: %v", err)
	}
	var dst interface{}
	switch m.Type {
	case ChatMessageType:
		dst = &ChatMessage{}
	case ClientInfoMessageType:
		dst = &ClientInfoMessage{}
	case ConnectedClientsMessageType:
		dst = &ConnectedClientsMessage{}
	}
	err = json.Unmarshal(m.Body, dst)
	if err != nil {
		log.Fatalf("error unmarshaling Message.Body: %v", err)
	}
	return dst, nil
}
