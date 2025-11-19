package kafka

import (
	"context"
	"log"

	kafkago "github.com/segmentio/kafka-go"

	ircevents "github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/irc_events"
)

func KafkaProducer(ctx context.Context, writer MessageWriter, parseCh <-chan ircevents.Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-parseCh:
			value, err := evt.Marshal()
			if err != nil {
				log.Println("marshal error:", err)
				continue
			}
			msg := kafkago.Message{
				Key:   []byte(evt.Key()),
				Value: value,
			}
			if err := writer.WriteMessages(ctx, msg); err != nil {
				log.Println("kafka write error:", err)
			}
		}
	}
}
