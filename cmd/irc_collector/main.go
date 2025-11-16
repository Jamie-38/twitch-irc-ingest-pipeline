package main

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"golang.org/x/sync/errgroup"

	channelrecord "github.com/Jamie-38/stream-pipeline/internal/channel_record"
	"github.com/Jamie-38/stream-pipeline/internal/config"
	"github.com/Jamie-38/stream-pipeline/internal/httpapi"
	ircevents "github.com/Jamie-38/stream-pipeline/internal/irc_events"
	kstream "github.com/Jamie-38/stream-pipeline/internal/kafka"
	"github.com/Jamie-38/stream-pipeline/internal/oauth"
	"github.com/Jamie-38/stream-pipeline/internal/observe"
	"github.com/Jamie-38/stream-pipeline/internal/scheduler"
	"github.com/Jamie-38/stream-pipeline/internal/types"
)

func main() {
	lg := observe.C("irc_collector")

	if err := config.LoadEnv(); err != nil {
		lg.Warn("env file not loaded", "err", err)
	}

	account, err := config.LoadAccount(os.Getenv("ACCOUNTS_PATH"))
	if err != nil {
		lg.Error("load account", "err", err, "path", os.Getenv("ACCOUNTS_PATH"))
		os.Exit(1)
	}
	selfLogin := strings.ToLower(account.User)

	token, err := oauth.LoadTokenJSON(os.Getenv("TOKENS_PATH"))
	if err != nil {
		lg.Error("load token", "err", err, "path", os.Getenv("TOKENS_PATH"))
		os.Exit(1)
	}

	// ctx canceled by signal
	root, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// pipeline context derives from root
	g, ctx := errgroup.WithContext(root)

	// pipeline channels
	controlCh := make(chan types.IRCCommand, 100)
	rectifierOutCh := make(chan types.IRCCommand, 100)
	membershipCh := make(chan types.MembershipEvent, 100)
	writerCh := make(chan string, 100)
	readerCh := make(chan string, 1000)
	parseCh := make(chan ircevents.Event, 1000)

	// connect (fail fast before goroutines)
	lg.Info("starting", "nick", account.Nick)

	conn, err := TwitchWebsocket(ctx, token.AccessToken, account.Nick, os.Getenv("TWITCH_IRC_URI"))
	if err != nil {
		lg.Error("websocket connect failed", "err", err, "uri", os.Getenv("TWITCH_IRC_URI"))
		os.Exit(1)
	}
	defer conn.Close()

	lg.Info("connected", "uri", os.Getenv("TWITCH_IRC_URI"))

	// Build JSON controller (single writer), consuming HTTP intents from controlCh.
	ctl, err := channelrecord.NewController(os.Getenv("CHANNELS_PATH"), account.Nick, controlCh)
	if err != nil {
		observe.C("channelrecord").Error("init controller", "err", err, "path", os.Getenv("CHANNELS_PATH"))
		os.Exit(1)
	}

	// kafka writer (lifecycle tied to main)
	w := kstream.NewWriter(os.Getenv("KAFKA_BROKERS"), os.Getenv("KAFKA_TOPIC"))
	defer w.Close()

	// all stages run under errgroup

	// Channels controller
	g.Go(func() error { return ctl.Run(ctx) })

	// HTTP control plane
	g.Go(func() error { return httpapi.Run(ctx, controlCh) })

	// Channel rectifier
	cfg := channelrecord.NewDefaultConfig()
	g.Go(func() error {
		return channelrecord.Run(ctx, ctl, membershipCh, rectifierOutCh, cfg)
	})

	// IRC control scheduler (JOIN/PART -> writerCh)
	g.Go(func() error {
		scheduler.Control_scheduler(ctx, rectifierOutCh, writerCh)
		return nil
	})

	// IRC socket reader -> readerCh (and PING signals -> writerCh)
	g.Go(func() error { return StartReader(ctx, conn, writerCh, readerCh) })

	// Single writer to the socket
	g.Go(func() error { return IRCWriter(ctx, conn, writerCh) })

	// Parser: readerCh -> parseCh
	g.Go(func() error {
		ClassifyLine(ctx, readerCh, parseCh, membershipCh, selfLogin)
		return nil
	})

	// Kafka producer: parseCh -> Kafka
	g.Go(func() error {
		kstream.KafkaProducer(ctx, w, parseCh)
		return nil
	})

	// wait for first error or signal
	if err := g.Wait(); err != nil {
		lg.Error("fatal pipeline error", "err", err)
	} else {
		lg.Info("shutdown complete")
	}
}
