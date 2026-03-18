package httpapi

import (
	"context"
	"encoding/json"
	"fmt"

	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/healthcheck"
	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/observe"
	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/types"
)

func (api *APIController) Join(w http.ResponseWriter, r *http.Request) {
	ch := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("channel")))
	if ch == "" {
		api.lg.Warn("join request missing channel parameter", "remote", r.RemoteAddr)
		http.Error(w, "Missing channel parameter", http.StatusBadRequest)
		return
	}

	api.lg.Info("enqueue join", "channel", ch, "remote", r.RemoteAddr)
	api.ControlCh <- types.IRCCommand{Op: "JOIN", Channel: "#" + ch}
	_, _ = w.Write([]byte("Queued join for channel: " + ch))
}

func (api *APIController) Part(w http.ResponseWriter, r *http.Request) {
	ch := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("channel")))
	if ch == "" {
		api.lg.Warn("part request missing channel parameter", "remote", r.RemoteAddr)
		http.Error(w, "Missing channel parameter", http.StatusBadRequest)
		return
	}
	api.lg.Info("enqueue part", "channel", ch, "remote", r.RemoteAddr)
	api.ControlCh <- types.IRCCommand{Op: "PART", Channel: "#" + ch}
	_, _ = w.Write([]byte("Queued part for channel: " + ch))
}

func (api *APIController) Channels(w http.ResponseWriter, r *http.Request) {
	version, channels, updatedAt, account := api.SnapshotReader.Snapshot()

	resp := struct {
		Account   string    `json:"account"`
		Version   uint64    `json:"version"`
		UpdatedAt time.Time `json:"updated_at"`
		Channels  []string  `json:"channels"`
	}{
		Account:   account,
		Version:   version,
		UpdatedAt: updatedAt,
		Channels:  channels,
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		api.lg.Error("encode channels response failed", "err", err, "remote", r.RemoteAddr)
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

func Run(ctx context.Context, controlCh chan types.IRCCommand, snapshotReader ChannelSnapshotReader) error {
	lg := observe.C("http_api")
	api := &APIController{
		ControlCh:      controlCh,
		SnapshotReader: snapshotReader,
		lg:             lg,
	}

	mux := http.NewServeMux()
	probe := healthcheck.New("http_api")
	probe.Register(mux)
	probe.SetNotReady()

	mux.HandleFunc("/join", api.Join)
	mux.HandleFunc("/part", api.Part)
	mux.HandleFunc("/channels", api.Channels)

	host := strings.TrimSpace(os.Getenv("HTTP_API_HOST"))
	if host == "" {
		return fmt.Errorf("HTTP_API_HOST missing")
	}
	portEnv := strings.TrimSpace(os.Getenv("HTTP_API_PORT"))
	if portEnv == "" {
		return fmt.Errorf("HTTP_API_PORT missing")
	}
	port, err := strconv.Atoi(portEnv)
	if err != nil {
		return fmt.Errorf("HTTP_API_PORT parse error")
	}
	if port <= 1 || port >= 65535 {
		return fmt.Errorf("HTTP_API_PORT out of bounds")
	}

	address := net.JoinHostPort(host, portEnv)

	srv := &http.Server{
		Addr:              address,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ln, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("http_api: listen error on %s: %w", address, err)
	}

	errCh := make(chan error, 1)
	go func() {
		lg.Info("listening", "address", address)
		probe.SetReady()
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		lg.Info("shutdown requested")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			_ = srv.Close()
			return err
		}
		lg.Info("shutdown complete")
		return nil
	case err := <-errCh:
		return err
	}
}
