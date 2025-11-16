package observe

import (
	"log/slog"
	"os"
)

var base = newLogger()

func newLogger() *slog.Logger {
	level := slog.LevelInfo
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		switch v {
		case "DEBUG", "debug":
			level = slog.LevelDebug
		case "WARN", "warn":
			level = slog.LevelWarn
		case "ERROR", "error":
			level = slog.LevelError
		}
	}
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	return slog.New(h).With(
		slog.String("service", "stream-pipeline"),
		slog.String("env", os.Getenv("APP_ENV")),
	)
}

func L() *slog.Logger { return base }

func C(component string) *slog.Logger {
	return base.With(slog.String("component", component))
}
