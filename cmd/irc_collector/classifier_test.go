package main

import (
	"context"
	"testing"
	"time"

	ircevents "github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/irc_events"
	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/types"
)

type clsRig struct {
	ctx    context.Context
	cancel context.CancelFunc
	in     chan string
	out    chan ircevents.Event
	memb   chan types.MembershipEvent
}

func newRig(self string) *clsRig {
	ctx, cancel := context.WithCancel(context.Background())
	r := &clsRig{
		ctx:    ctx,
		cancel: cancel,
		in:     make(chan string, 8),
		out:    make(chan ircevents.Event, 8),
		memb:   make(chan types.MembershipEvent, 8),
	}
	go ClassifyLine(ctx, r.in, r.out, r.memb, self)
	return r
}

func (r *clsRig) close() {
	r.cancel()
	close(r.in)
}

func recvEvt[T any](ch <-chan T) (v T, ok bool) {
	select {
	case v = <-ch:
		return v, true
	case <-time.After(100 * time.Millisecond):
		return v, false
	}
}

func TestClassifier_PrivMsg_Full(t *testing.T) {
	r := newRig("selfuser")
	defer r.close()

	line := "@user-id=123;room-id=999;color=\\:blue\\;;badges=subscriber/3 :bob!bob@bob.tmi.twitch.tv PRIVMSG #chess :hello\\sworld!"
	r.in <- line

	ev, ok := recvEvt(r.out)
	if !ok {
		t.Fatal("no event emitted")
	}
	pm, ok := ev.(ircevents.PrivMsg)
	if !ok {
		t.Fatalf("expected PrivMsg, got %T", ev)
	}

	if pm.UserID != "123" || pm.ChannelID != "999" {
		t.Fatalf("wrong ids: %+v", pm)
	}
	if pm.UserLogin != "bob" || pm.ChannelLogin != "chess" {
		t.Fatalf("wrong logins: %+v", pm)
	}
	if pm.Text != "hello\\sworld!" {
		t.Fatalf("unexpected text (trailing is not IRCv3-escaped): %q", pm.Text)
	}
}

func TestClassifier_PrivMsg_NoTags_Fallback(t *testing.T) {
	r := newRig("selfuser")
	defer r.close()

	line := ":Alice!alice@tmi.twitch.tv PRIVMSG #SpeedRun :Go fast"
	r.in <- line

	ev, ok := recvEvt(r.out)
	if !ok {
		t.Fatal("no event")
	}
	pm := ev.(ircevents.PrivMsg)

	// IDs absent, but logins captured (lowercased)
	if pm.UserID != "" || pm.ChannelID != "" {
		t.Fatalf("unexpected ids: %+v", pm)
	}
	if pm.UserLogin != "alice" || pm.ChannelLogin != "speedrun" {
		t.Fatalf("fallback logins wrong: %+v", pm)
	}
	if pm.Text != "Go fast" {
		t.Fatalf("text mismatch: %q", pm.Text)
	}
}

func TestClassifier_PrivMsg_Malformed_Skipped(t *testing.T) {
	r := newRig("selfuser")
	defer r.close()

	// Missing trailing part after " :"
	r.in <- ":bob!bob@tmi PRIVMSG #chess"
	if _, ok := recvEvt(r.out); ok {
		t.Fatal("expected no event for malformed PRIVMSG")
	}
}

func TestClassifier_Membership_SelfOnly(t *testing.T) {
	r := newRig("me")
	defer r.close()

	// Another user JOIN - ignored
	r.in <- ":alice!alice@tmi.twitch.tv JOIN #chess"
	if _, ok := recvEvt(r.memb); ok {
		t.Fatal("non-self JOIN should not emit membership event")
	}

	// Self JOIN emits
	r.in <- ":me!me@tmi.twitch.tv JOIN #chess"
	ev, ok := recvEvt(r.memb)
	if !ok {
		t.Fatal("expected membership join")
	}
	if ev.Op != "JOIN" || ev.Channel != "#chess" {
		t.Fatalf("wrong membership: %+v", ev)
	}

	// Self PART emits
	r.in <- ":me!me@tmi.twitch.tv PART chess" // missing '#' accepted by normaliser in writer
	ev, ok = recvEvt(r.memb)
	if !ok {
		t.Fatal("expected membership part")
	}
	if ev.Op != "PART" || ev.Channel != "#chess" {
		t.Fatalf("wrong membership: %+v", ev)
	}
}

func TestParseTagsAndUnescape(t *testing.T) {
	m := parseTags("a=1;b=hello\\sworld;c=\\:;flagonly")
	if m["a"] != "1" {
		t.Fatal("a")
	}
	if m["b"] != "hello world" {
		t.Fatalf("b=%q", m["b"])
	}
	if m["c"] != ":" {
		t.Fatalf("c=%q", m["c"])
	}
	if m["flagonly"] != "1" {
		t.Fatal("flagonly")
	}
}

func FuzzUnescapeIRCv3(f *testing.F) {
	seeds := []string{``, `\\`, `\s\:\;`, `a\b\c`, `hello\nworld`}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		_ = unescapeIRCv3(s) // must not panic
	})
}

func TestLoginFromPrefix(t *testing.T) {
	cases := map[string]string{
		"alice!alice@tmi.twitch.tv": "alice",
		"tmi.twitch.tv":             "tmi.twitch.tv",
		"":                          "",
	}
	for in, want := range cases {
		if got := loginFromPrefix(in); got != want {
			t.Fatalf("in=%q got=%q want=%q", in, got, want)
		}
	}
}
