package scheduler

import (
	"context"
	"fmt"

	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/observe"
	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/types"
)

func Control_scheduler(ctx context.Context, controlCh <-chan types.IRCCommand, writerCh chan<- string) {
	lg := observe.C("scheduler")

	send := func(line string, channel string) {
		select {
		case writerCh <- line:
			// sent
		case <-ctx.Done():
			lg.Info("stopping before send", "channel", channel)
		}
	}

	for {
		select {
		case <-ctx.Done():
			lg.Info("stopping", "reason", "context_canceled")
			return

		case cmd, ok := <-controlCh:
			if !ok {
				lg.Info("stopping", "reason", "control_channel_closed")
				return
			}

			switch cmd.Op {
			case "JOIN":
				lg.Debug("forwarded JOIN", "channel", cmd.Channel)
				send(fmt.Sprintf("JOIN %s\r\n", cmd.Channel), cmd.Channel)

			case "PART":
				lg.Debug("forwarded PART", "channel", cmd.Channel)
				send(fmt.Sprintf("PART %s\r\n", cmd.Channel), cmd.Channel)

			default:
				lg.Warn("unknown IRC command", "op", cmd.Op, "channel", cmd.Channel)
			}
		}
	}
}
