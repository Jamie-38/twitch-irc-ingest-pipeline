package httpapi

import (
	"log/slog"
	"time"

	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/types"
)

type ChannelSnapshotReader interface {
	Snapshot() (version uint64, channels []string, updatedAt time.Time, account string)
}

type APIController struct {
	ControlCh      chan types.IRCCommand
	SnapshotReader ChannelSnapshotReader
	lg             *slog.Logger
}
