package channelrecord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/observe"
	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/types"
)

type Controller struct {
	path            string // path to channels.json
	account         string // validated account name
	schema          int    // schema version
	controlCh       <-chan types.IRCCommand
	updatesCh       chan struct{}
	mu              sync.RWMutex
	snap            snapshot // immutable view for readers
	writeDebounceMs int      // debounce window
	lg              *slog.Logger
}

type snapshot struct {
	Version   uint64
	Account   string
	UpdatedAt time.Time
	Channels  []string
}

func NewController(path string, expectedAccount string, controlCh <-chan types.IRCCommand) (*Controller, error) {
	lg := observe.
		C("channelrecord").
		With("account", expectedAccount, "path", path)

	if path == "" {
		return nil, errors.New("channelrecord: empty path")
	}
	if expectedAccount == "" {
		return nil, errors.New("channelrecord: empty expectedAccount")
	}

	c := &Controller{
		path:            path,
		account:         expectedAccount,
		schema:          1,
		controlCh:       controlCh,
		updatesCh:       make(chan struct{}, 1),
		writeDebounceMs: 150,
		lg:              lg,
	}

	onDisk, err := loadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("channelrecord: load channels file %q: %w", path, err)
	}

	var desired map[string]struct{}
	if err == nil {
		if onDisk.Account != "" && onDisk.Account != expectedAccount {
			return nil, fmt.Errorf("channelrecord: channels file account %q != expected %q",
				onDisk.Account, expectedAccount)
		}
		desired = sliceToSet(onDisk.Channels)
		c.schema = onDisk.Schema
		lg.Debug("loaded channels file", "schema", c.schema, "channels", len(desired))
	} else {
		desired = make(map[string]struct{})
		lg.Debug("no existing channels file; will initialize")
	}

	// Build initial immutable snapshot
	chans := setToSortedSlice(desired)
	c.snap = snapshot{
		Version:   1,
		Account:   expectedAccount,
		UpdatedAt: time.Now().UTC(),
		Channels:  chans,
	}

	if errors.Is(err, os.ErrNotExist) {
		if err := c.writeFile(c.snap); err != nil {
			return nil, fmt.Errorf("channelrecord: initialize channels file %q: %w", path, err)
		}
		lg.Info("initialized channels file", "channels", len(chans))
	}

	lg.Info("controller ready",
		"version", c.snap.Version, "channels", len(chans))
	return c, nil
}

func (c *Controller) Run(ctx context.Context) error {
	lg := c.lg

	desired := sliceToSet(c.readSnap().Channels)
	version := c.readSnap().Version

	dirty := false
	var debounce <-chan time.Time

	for {
		select {
		case <-ctx.Done():
			s := c.readSnap()
			lg.Info("controller stopping",
				"reason", "context_canceled",
				"version", s.Version,
				"channels_count", len(s.Channels),
				"path", c.path,
				"account", c.account,
			)

			return ctx.Err()

		case cmd := <-c.controlCh:
			ch, ok := normalizeChannel(cmd.Channel)
			if !ok {
				lg.Debug("dropping invalid command", "op", cmd.Op, "raw_channel", cmd.Channel)
				continue
			}

			switch cmd.Op {
			case "JOIN":
				if _, exists := desired[ch]; !exists {
					desired[ch] = struct{}{}
					dirty = true
					lg.Info("desired add", "channel", ch)
					if debounce == nil {
						debounce = time.After(time.Duration(c.writeDebounceMs) * time.Millisecond)
					}
				}
			case "PART":
				if _, exists := desired[ch]; exists {
					delete(desired, ch)
					dirty = true
					lg.Info("desired remove", "channel", ch)
					if debounce == nil {
						debounce = time.After(time.Duration(c.writeDebounceMs) * time.Millisecond)
					}
				}
			default:
				lg.Debug("unknown op", "op", cmd.Op)
			}

		case <-debounce:
			if dirty {
				version++
				newSnap := snapshot{
					Version:   version,
					Account:   c.account,
					UpdatedAt: time.Now().UTC(),
					Channels:  setToSortedSlice(desired),
				}

				if err := c.writeFile(newSnap); err != nil {
					return err
				}
				c.writeSnap(newSnap)
				c.nonBlockingNotify()
				lg.Info("persisted snapshot", "version", version, "channels", len(newSnap.Channels))
			}
			dirty = false
			debounce = nil
		}
	}
}

func (c *Controller) Snapshot() (version uint64, channels []string, updatedAt time.Time, account string) {
	s := c.readSnap()
	cp := make([]string, len(s.Channels))
	copy(cp, s.Channels)
	return s.Version, cp, s.UpdatedAt, s.Account
}

func (c *Controller) Updates() <-chan struct{} {
	return c.updatesCh
}

func (c *Controller) readSnap() snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.snap
}

func (c *Controller) writeSnap(s snapshot) {
	c.mu.Lock()
	c.snap = s
	c.mu.Unlock()
}

func (c *Controller) nonBlockingNotify() {
	select {
	case c.updatesCh <- struct{}{}:
	default:
		// drop if full
	}
}

func normalizeChannel(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	var s string
	for i := 0; i < len(raw); i++ {
		if raw[i] != ' ' && raw[i] != '\t' && raw[i] != '\n' && raw[i] != '\r' {
			s = raw[i:]
			break
		}
	}
	if s == "" {
		return "", false
	}

	b := make([]byte, 0, len(s)+1)
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch >= 'A' && ch <= 'Z' {
			ch = ch - 'A' + 'a'
		}
		b = append(b, ch)
	}
	if b[0] != '#' {
		b = append([]byte{'#'}, b...)
	}
	return string(b), true
}

func sliceToSet(xs []string) map[string]struct{} {
	m := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		if ch, ok := normalizeChannel(x); ok {
			m[ch] = struct{}{}
		}
	}
	return m
}

func setToSortedSlice(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (c *Controller) writeFile(s snapshot) error {
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	tmp := c.path + ".tmp"

	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open tmp: %w", err)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	onDisk := types.Channels{
		Schema:    c.schema,
		Account:   c.account,
		UpdatedAt: s.UpdatedAt,
		Channels:  s.Channels,
	}
	if err := enc.Encode(&onDisk); err != nil {
		_ = f.Close()
		return fmt.Errorf("encode json: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("fsync tmp: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close tmp: %w", err)
	}
	if err := os.Rename(tmp, c.path); err != nil {
		return fmt.Errorf("rename tmp→final: %w", err)
	}
	return nil
}

func loadFile(path string) (types.Channels, error) {
	var v types.Channels
	b, err := os.ReadFile(path)
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return v, fmt.Errorf("decode %s: %w", path, err)
	}
	return v, nil
}
