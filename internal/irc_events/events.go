package ircevents

import "encoding/json"

type Event interface {
	Kind() string
	Key() string
	Marshal() ([]byte, error)
}

type PrivMsg struct {
	UserID       string
	UserLogin    string
	ChannelID    string
	ChannelLogin string
	Text         string
}

type JoinPart struct {
	UserID    string
	ChannelID string
	Op        string
}

func (msg PrivMsg) Kind() string {
	return "privmsg"
}

func (msg PrivMsg) Key() string {
	return msg.ChannelID
}

func (msg PrivMsg) Marshal() ([]byte, error) {
	return json.Marshal(msg)
}
