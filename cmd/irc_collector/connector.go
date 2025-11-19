package main

import (
	"context"
	"fmt"

	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/observe"
	"github.com/gorilla/websocket"
)

func TwitchWebsocket(ctx context.Context, token, username, uri string) (*websocket.Conn, error) {
	lg := observe.C("connector").With("user", username, "uri", uri)

	d := websocket.Dialer{}
	conn, _, err := d.Dial(uri, nil)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	lg.Debug("websocket dialed")

	write := func(s string) error {
		return conn.WriteMessage(websocket.TextMessage, []byte(s))
	}

	// 1) Auth
	if err := write(fmt.Sprintf("PASS oauth:%s\r\n", token)); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("pass: %w", err)
	}
	lg.Debug("sent PASS")

	if err := write(fmt.Sprintf("NICK %s\r\n", username)); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("nick: %w", err)
	}
	lg.Debug("sent NICK")

	// 2) Capabilities (batched)
	if err := write("CAP REQ :twitch.tv/tags twitch.tv/commands twitch.tv/membership\r\n"); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("cap req: %w", err)
	}
	lg.Debug("requested capabilities")

	return conn, nil
}
