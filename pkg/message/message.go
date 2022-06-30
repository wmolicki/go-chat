package message

import (
	"encoding/json"
	"fmt"
)

const (
	UserMessage int = iota
	ConnectedClientsMsg
	ClientInfoMsg
)

type Envelope[T any] struct {
	Type int
	Msg  T
}

type Message struct {
	Text     string `json:"text"`
	Username string `json:"username"`
}

func Encode(username string, message string) ([]byte, error) {
	m := make(map[string]string)
	m["username"] = username
	m["text"] = message
	encoded, err := json.Marshal(m)
	if err != nil {
		return []byte{}, err
	}
	return encoded, nil
}

func Decode(payload []byte) (map[string]string, error) {
	m := make(map[string]string)
	err := json.Unmarshal(payload, &m)
	if err != nil {
		return m, fmt.Errorf("error unmarshaling payload: %v", err)
	}
	return m, nil
}
