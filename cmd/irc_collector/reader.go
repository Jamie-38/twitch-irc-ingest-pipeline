package main

import (
	"context"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/observe"
)

func StartReader(ctx context.Context, conn *websocket.Conn, writerCh chan<- string, readCh chan<- string) error {
	lg := observe.C("reader")

	// Ensure ReadMessage unblocks when ctx is cancelled.
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				lg.Info("reader stopping", "reason", "context_canceled")
				return ctx.Err()
			}
			lg.Warn("socket read failed", "err", err)
			return err
		}

		for _, line := range strings.Split(string(payload), "\r\n") {
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "PING") {
				select {
				case writerCh <- "PONG :tmi.twitch.tv\r\n":
				case <-ctx.Done():
					return ctx.Err()
				}
				continue
			}
			select {
			case readCh <- line:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}
