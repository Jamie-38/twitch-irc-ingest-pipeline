package main

import (
	"context"

	"github.com/gorilla/websocket"

	"github.com/Jamie-38/stream-pipeline/internal/observe"
)

func IRCWriter(ctx context.Context, conn *websocket.Conn, writerCh <-chan string) error {
	lg := observe.C("writer")
	for {
		select {
		case line := <-writerCh:
			if err := conn.WriteMessage(websocket.TextMessage, []byte(line)); err != nil {
				lg.Error("socket write failed", "err", err)
				return err
			}
		case <-ctx.Done():
			lg.Info("writer stopping")
			return ctx.Err()
		}
	}
}
