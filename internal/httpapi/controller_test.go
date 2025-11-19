package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/observe"
	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/types"
)

func TestJoinEnqueuesLowercasedHashChannel(t *testing.T) {
	ch := make(chan types.IRCCommand, 1)
	api := &APIController{
		ControlCh: ch,
		lg:        observe.C("httpapi_test"),
	}

	req := httptest.NewRequest("GET", "/join?channel=Chess", nil)
	w := httptest.NewRecorder()

	api.Join(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	select {
	case cmd := <-ch:
		if cmd.Op != "JOIN" || cmd.Channel != "#chess" {
			t.Fatalf("enqueued = %+v, want JOIN #chess", cmd)
		}
	default:
		t.Fatal("no command enqueued")
	}
}

func TestJoinMissingParam(t *testing.T) {
	ch := make(chan types.IRCCommand, 1)
	api := &APIController{
		ControlCh: ch,
		lg:        observe.C("httpapi_test"),
	}

	req := httptest.NewRequest("GET", "/join", nil)
	w := httptest.NewRecorder()

	api.Join(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if len(ch) != 0 {
		t.Fatal("should not enqueue on error")
	}
}

func TestPartEnqueuesLowercasedHashChannel(t *testing.T) {
	ch := make(chan types.IRCCommand, 1)
	api := &APIController{ControlCh: ch, lg: observe.C("httpapi_test")}

	req := httptest.NewRequest("GET", "/part?channel=Chess", nil)
	w := httptest.NewRecorder()

	api.Part(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	select {
	case cmd := <-ch:
		if cmd.Op != "PART" || cmd.Channel != "#chess" {
			t.Fatalf("enqueued = %+v, want PART #chess", cmd)
		}
	default:
		t.Fatal("no command enqueued")
	}
}

func TestPartMissingParam(t *testing.T) {
	ch := make(chan types.IRCCommand, 1)
	api := &APIController{ControlCh: ch, lg: observe.C("httpapi_test")}

	req := httptest.NewRequest("GET", "/part", nil)
	w := httptest.NewRecorder()

	api.Part(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if len(ch) != 0 {
		t.Fatal("should not enqueue on error")
	}
}
