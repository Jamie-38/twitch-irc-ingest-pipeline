package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/segmentio/kafka-go"
)

func main() {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		log.Fatal("KAFKA_BROKERS is empty")
	}
	topic := os.Getenv("KAFKA_TOPIC")
	if topic == "" {
		log.Fatal("KAFKA_TOPIC is empty")
	}
	groupID := os.Getenv("KAFKA_GROUPID")
	if topic == "" {
		log.Fatal("KAFKA_GROUPID is empty")
	}

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{brokers},
		GroupID:  groupID,
		Topic:    topic,
		MaxBytes: 10e6, // 10MB
	})
	defer func() {
		if err := r.Close(); err != nil {
			log.Printf("failed to close reader: %v", err)
		}
	}()

	for {
		m, err := r.ReadMessage(context.Background())
		if err != nil {
			log.Printf("read error: %v", err)
			return
		}
		fmt.Printf(
			"message at topic/partition/offset %v/%v/%v: %s = %s\n",
			m.Topic, m.Partition, m.Offset, string(m.Key), string(m.Value),
		)
	}
}
