package kafka

import (
	"context"
	"strings"

	kafkago "github.com/segmentio/kafka-go"
)

type MessageWriter interface {
	WriteMessages(ctx context.Context, msgs ...kafkago.Message) error
	Close() error
}

func NewWriter(brokersCSV, topic string) *kafkago.Writer {
	parts := strings.Split(brokersCSV, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return &kafkago.Writer{
		Addr:     kafkago.TCP(parts...),
		Topic:    topic,
		Balancer: &kafkago.LeastBytes{},
	}
}
