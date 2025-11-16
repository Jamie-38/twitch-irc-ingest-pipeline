package healthcheck

import (
	"net/http"
	"sync/atomic"
	"time"

	"log/slog"

	"github.com/Jamie-38/stream-pipeline/internal/observe"
)

type Probe struct {
	ready int32 // 0 = not ready, 1 = ready
	lg    *slog.Logger
}

func New(component string) *Probe {
	return &Probe{
		lg: observe.C("healthcheck").With("component", component),
	}
}

func (p *Probe) Register(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if atomic.LoadInt32(&p.ready) == 1 {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ready"))
			return
		}
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	})
}

func (p *Probe) SetReady() {
	prev := atomic.SwapInt32(&p.ready, 1)
	if prev != 1 {
		p.lg.Info("readiness transition", "from", "not_ready", "to", "ready", "ts", time.Now().UTC())
	}
}

func (p *Probe) SetNotReady() {
	prev := atomic.SwapInt32(&p.ready, 0)
	if prev != 0 {
		p.lg.Info("readiness transition", "from", "ready", "to", "not_ready", "ts", time.Now().UTC())
	}
}
