package channelrecord

import (
	"context"
	"strings"
	"time"

	"log/slog"

	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/observe"
	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/types"
)

type DesiredSnapshot interface {
	Snapshot() (version uint64, channels []string, updatedAt time.Time, account string)
	Updates() <-chan struct{}
}

type Config struct {
	TokensPerSecond float64
	Burst           int
	JoinTimeout     time.Duration
	BackoffMin      time.Duration
	BackoffMax      time.Duration
	Tick            time.Duration
}

func NewDefaultConfig() Config {
	return Config{
		TokensPerSecond: 0.5,
		Burst:           2,
		JoinTimeout:     30 * time.Second,
		BackoffMin:      2 * time.Second,
		BackoffMax:      60 * time.Second,
		Tick:            1 * time.Second,
	}
}

func Run(ctx context.Context, desired DesiredSnapshot, events <-chan types.MembershipEvent, out chan<- types.IRCCommand, cfg Config) error {
	_, _, _, acct := desired.Snapshot()
	lg := observe.C("rectifier").With(
		"account", acct,
		"tokens_per_sec", cfg.TokensPerSecond,
		"burst", cfg.Burst,
		"join_timeout_s", cfg.JoinTimeout.Seconds(),
		"backoff_min_s", cfg.BackoffMin.Seconds(),
		"backoff_max_s", cfg.BackoffMax.Seconds(),
		"tick_ms", cfg.Tick.Milliseconds(),
	)

	r := &reconciler{
		desired:      desired,
		events:       events,
		out:          out,
		cfg:          cfg,
		state:        make(map[string]*chanState),
		tokenBucket:  newBucket(cfg.TokensPerSecond, cfg.Burst, realClock{}),
		lastDesiredV: 0,
		lg:           lg,
		clk:          realClock{},
	}

	lg.Info("rectifier starting")
	err := r.loop(ctx)
	if err != nil {
		lg.Error("rectifier stopped", "err", err)
	} else {
		lg.Info("rectifier stopped")
	}
	return err
}

type phase int

func (p phase) String() string {
	switch p {
	case Idle:
		return "Idle"
	case Joining:
		return "Joining"
	case Joined:
		return "Joined"
	case Parting:
		return "Parting"
	case Error:
		return "Error"
	default:
		return "Unknown"
	}
}

const (
	Idle phase = iota
	Joining
	Joined
	Parting
	Error
)

type chanState struct {
	want      bool
	have      bool
	phase     phase
	lastTry   time.Time
	deadline  time.Time
	backoff   time.Duration
	nextTryAt time.Time
}

type reconciler struct {
	desired      DesiredSnapshot
	events       <-chan types.MembershipEvent
	out          chan<- types.IRCCommand
	cfg          Config
	state        map[string]*chanState
	tokenBucket  *bucket
	lastDesiredV uint64
	lg           *slog.Logger
	clk          Clock
}

func (r *reconciler) loop(ctx context.Context) error {
	tick := time.NewTicker(r.cfg.Tick)
	defer tick.Stop()

	updates := r.desired.Updates()

	for {
		select {
		case <-ctx.Done():
			r.lg.Info("rectifier stopping: context canceled")
			return ctx.Err()

		case <-tick.C:
			r.observeDesired()
			r.reconcile(r.clk.Now())

		case <-updates:
			r.lg.Debug("desired snapshot updated")
			r.observeDesired()
			r.reconcile(r.clk.Now())

		case evt := <-r.events:
			r.lg.Debug("membership event", "op", evt.Op, "channel", evt.Channel)
			r.observeEvent(evt)
			r.reconcile(r.clk.Now())
		}
	}
}

func (r *reconciler) observeDesired() {
	v, chans, _, _ := r.desired.Snapshot()
	if v == r.lastDesiredV {
		return
	}
	r.lg.Info("desired set changed", "version", v, "channels", len(chans))
	for name, s := range r.state {
		s.want = false
		_ = name
	}
	for _, ch := range chans {
		s := r.ensure(ch)
		s.want = true
	}
	r.lastDesiredV = v
}

func (r *reconciler) observeEvent(evt types.MembershipEvent) {
	switch evt.Op {
	case "JOIN":
		ch := strings.ToLower(evt.Channel)
		if !strings.HasPrefix(ch, "#") {
			ch = "#" + ch
		}
		s := r.ensure(ch)
		if !s.have {
			s.have = true
			s.phase = Joined
			r.lg.Info("join confirmed", "channel", ch)
		}
	case "PART":
		ch := strings.ToLower(evt.Channel)
		if !strings.HasPrefix(ch, "#") {
			ch = "#" + ch
		}
		s := r.ensure(ch)
		if s.have {
			s.have = false
			s.phase = Idle
			r.lg.Info("part confirmed", "channel", ch)
		}
	default:
		// ignore
	}
}

func (r *reconciler) reconcile(now time.Time) {

	for name, s := range r.state {
		if s.want {
			continue
		}
		if s.have && (s.phase == Idle || s.phase == Joined || (s.phase == Error && now.After(s.nextTryAt))) {
			r.lg.Debug("trying PART", "channel", name, "phase", s.phase.String())
			if r.trySend(now, "PART", name, s) {
				continue
			}
		}

		r.maybeTimeout(now, s)
	}

	for name, s := range r.state {
		if !s.want {
			continue
		}
		if !s.have && (s.phase == Idle || s.phase == Error && now.After(s.nextTryAt)) {
			r.lg.Debug("trying JOIN", "channel", name, "phase", s.phase.String())
			if r.trySend(now, "JOIN", name, s) {
				continue
			}
		}
		r.maybeTimeout(now, s)
	}
}

func (r *reconciler) trySend(now time.Time, op string, channel string, s *chanState) bool {
	if !r.tokenBucket.take(now) {
		r.lg.Debug("rate-limited; skipping for now", "op", op, "channel", channel)
		return false
	}

	select {
	case r.out <- types.IRCCommand{Op: op, Channel: channel}:
		s.lastTry = now
		s.deadline = now.Add(r.cfg.JoinTimeout)
		if op == "JOIN" {
			s.phase = Joining
			if s.backoff == 0 {
				s.backoff = r.cfg.BackoffMin
			}
		} else {
			s.phase = Parting
			if s.backoff == 0 {
				s.backoff = r.cfg.BackoffMin
			}
		}
		r.lg.Info("command emitted", "op", op, "channel", channel, "deadline_s", r.cfg.JoinTimeout.Seconds())
		return true
	default:
		r.tokenBucket.refund(now)
		r.lg.Warn("out channel full; command not emitted", "op", op, "channel", channel)
		return false
	}
}

func (r *reconciler) maybeTimeout(now time.Time, s *chanState) {
	if (s.phase == Joining || s.phase == Parting) && now.After(s.deadline) {
		op := s.phase.String()

		s.phase = Error
		s.nextTryAt = now.Add(s.backoff)

		r.lg.Info("operation timed out; scheduling retry",
			"phase", op,
			"next_try_in_s", s.backoff.Seconds(),
		)

		s.backoff *= 2
		if s.backoff > r.cfg.BackoffMax {
			s.backoff = r.cfg.BackoffMax
		}
	}
}

func (r *reconciler) ensure(ch string) *chanState {
	if st, ok := r.state[ch]; ok {
		return st
	}
	st := &chanState{
		want:    false,
		have:    false,
		phase:   Idle,
		backoff: r.cfg.BackoffMin,
	}
	r.state[ch] = st
	r.lg.Debug("created channel state", "channel", ch)
	return st
}

type bucket struct {
	rate       float64
	capacity   float64
	tokens     float64
	lastUpdate time.Time
	clk        Clock
}

func newBucket(tokensPerSec float64, burst int, clk Clock) *bucket {
	now := clk.Now()
	return &bucket{
		rate:       tokensPerSec,
		capacity:   float64(burst),
		tokens:     float64(burst),
		lastUpdate: now,
		clk:        clk,
	}
}

func (b *bucket) refill(now time.Time) {
	elapsed := now.Sub(b.lastUpdate).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * b.rate
		if b.tokens > b.capacity {
			b.tokens = b.capacity
		}
		b.lastUpdate = now
	}
}

func (b *bucket) take(now time.Time) bool {
	b.refill(now)
	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}
	return false
}

func (b *bucket) refund(now time.Time) {
	b.refill(now)
	b.tokens++
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
}
