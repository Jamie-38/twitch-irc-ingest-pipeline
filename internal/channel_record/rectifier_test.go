package channelrecord

import (
	"testing"
	"time"

	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/observe"
	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/types"
)

type desiredStub struct {
	v       uint64
	chs     []string
	t       time.Time
	acct    string
	updates chan struct{}
}

func newDesiredStub(acct string, chans []string, t time.Time) *desiredStub {
	ds := &desiredStub{
		v:       1,
		chs:     chans,
		t:       t,
		acct:    acct,
		updates: make(chan struct{}, 1),
	}
	ds.updates <- struct{}{}
	return ds
}

func (d *desiredStub) Snapshot() (uint64, []string, time.Time, string) {
	return d.v, append([]string(nil), d.chs...), d.t, d.acct
}

func (d *desiredStub) Updates() <-chan struct{} { return d.updates }

// test
func TestRectifier_JoinRetryAndConfirm(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	clk := newFakeClock(start)

	cfg := NewDefaultConfig()
	cfg.TokensPerSecond = 100
	cfg.Burst = 10
	cfg.JoinTimeout = 2 * time.Second
	cfg.BackoffMin = 1 * time.Second
	cfg.BackoffMax = 4 * time.Second
	cfg.Tick = 200 * time.Millisecond

	ds := newDesiredStub("me", []string{"#chess"}, clk.Now())
	events := make(chan types.MembershipEvent, 4)
	out := make(chan types.IRCCommand, 4)

	r := &reconciler{
		desired:      ds,
		events:       events,
		out:          out,
		cfg:          cfg,
		state:        make(map[string]*chanState),
		tokenBucket:  newBucket(cfg.TokensPerSecond, cfg.Burst, clk),
		lastDesiredV: 0,
		lg:           observe.C("rectifier_test"),
	}

	r.observeDesired()
	r.reconcile(clk.Now())

	select {
	case cmd := <-out:
		if cmd.Op != "JOIN" || cmd.Channel != "#chess" {
			t.Fatalf("expected JOIN #chess, got %+v", cmd)
		}
	default:
		t.Fatalf("expected a JOIN command to be emitted")
	}

	clk.Advance(cfg.JoinTimeout + time.Millisecond)
	r.reconcile(clk.Now())

	st := r.state["#chess"]
	if st == nil {
		t.Fatalf("channel state not created")
	}
	if st.phase != Error {
		t.Fatalf("expected phase=Error after timeout, got %v", st.phase)
	}
	if st.backoff != cfg.BackoffMin*2 {
		t.Fatalf("expected backoff=%v, got %v", cfg.BackoffMin*2, st.backoff)
	}

	// Advance to retry time and reconcile, should emit another JOIN
	retryDelay := st.nextTryAt.Sub(clk.Now())
	if retryDelay < 0 {
		retryDelay = 0
	}
	clk.Advance(retryDelay + time.Millisecond)
	r.reconcile(clk.Now())

	select {
	case cmd := <-out:
		if cmd.Op != "JOIN" || cmd.Channel != "#chess" {
			t.Fatalf("expected re-JOIN #chess, got %+v", cmd)
		}
	default:
		t.Fatalf("expected a re-JOIN command to be emitted")
	}

	// Now simulate the membership confirmation. phase becomes Joined
	r.observeEvent(types.MembershipEvent{Op: "JOIN", Channel: "#chess"})
	r.reconcile(clk.Now())

	st = r.state["#chess"]
	if st.phase != Joined {
		t.Fatalf("expected phase=Joined after membership event, got %v", st.phase)
	}
}

func TestRectifier_PartConfirmResetsHave(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	clk := newFakeClock(start)

	cfg := NewDefaultConfig()
	cfg.TokensPerSecond = 100
	cfg.Burst = 10
	cfg.JoinTimeout = 2 * time.Second
	cfg.BackoffMin = 1 * time.Second
	cfg.BackoffMax = 4 * time.Second
	cfg.Tick = 200 * time.Millisecond

	// Desired empty
	ds := newDesiredStub("me", []string{}, clk.Now())
	events := make(chan types.MembershipEvent, 4)
	out := make(chan types.IRCCommand, 4)

	r := &reconciler{
		desired:      ds,
		events:       events,
		out:          out,
		cfg:          cfg,
		state:        make(map[string]*chanState),
		tokenBucket:  newBucket(cfg.TokensPerSecond, cfg.Burst, clk),
		lastDesiredV: 0,
		lg:           observe.C("rectifier_test"),
		clk:          clk,
	}

	// Pretend we were joined already.
	s := r.ensure("#chess")
	s.want = false
	s.have = true
	s.phase = Joined

	r.reconcile(clk.Now())
	select {
	case cmd := <-out:
		if cmd.Op != "PART" || cmd.Channel != "#chess" {
			t.Fatalf("expected PART #chess, got %+v", cmd)
		}
	default:
		t.Fatalf("expected a PART command to be emitted")
	}

	// Simulate PART confirmation
	r.observeEvent(types.MembershipEvent{Op: "PART", Channel: "#chess"})
	if st := r.state["#chess"]; st == nil || st.have {
		t.Fatalf("expected have=false after PART confirm, got %+v", st)
	}
}
