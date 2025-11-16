package httpapi

import (
	"log/slog"

	"github.com/Jamie-38/stream-pipeline/internal/types"
)

type APIController struct {
	ControlCh chan types.IRCCommand
	lg        *slog.Logger
}
