package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/config"
	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/healthcheck"
	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/oauth"
	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/observe"
)

func main() {
	lg := observe.C("oauth_server")

	if err := config.LoadEnv(); err != nil {
		lg.Warn("env file not loaded", "err", err)
	}

	mux := http.NewServeMux()
	probe := healthcheck.New("oauth_server")
	probe.Register(mux)
	probe.SetNotReady()

	mux.HandleFunc("/", oauth.Index)
	mux.HandleFunc("/callback", oauth.Callback)

	port := os.Getenv("OAUTH_SERVER_PORT")
	if port == "" {
		lg.Error("missing required env var", "name", "OAUTH_SERVER_PORT")
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		lg.Error("listen error", "err", err, "port", port)
		os.Exit(1)
	}

	go func() {
		lg.Info("listening", "addr", ":"+port)
		probe.SetReady()
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			lg.Error("serve error", "err", err)
			os.Exit(1)
		}
	}()

	// Trap SIGINT/SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	<-stop
	lg.Info("shutdown signal received", "port", port)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		lg.Error("graceful shutdown did not complete", "err", err)

		if cerr := srv.Close(); cerr != nil {
			lg.Error("forced close error", "err", cerr)
		}
	} else {
		lg.Info("shut down cleanly", "port", port)
	}
}
