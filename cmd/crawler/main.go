package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/mazenh0/auroraseek/internal/kafka"
)

type Page struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

func main() {
	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	if kafkaBrokers == "" {
		kafkaBrokers = "localhost:9092"
		log.Printf("WARNING: KAFKA_BROKERS not set, using default: %s", kafkaBrokers)
	} else {
		log.Printf("Using KAFKA_BROKERS: %s", kafkaBrokers)
	}
	
	brokers := []string{kafkaBrokers}
	topic := getenv("KAFKA_TOPIC_PAGES", "pages")

	log.Printf("Connecting to Kafka at %v, topic: %s", brokers, topic)

	p := kafka.NewProducer(brokers, topic)
	defer p.Close()

	seeds := []Page{
		{
			ID:    "1",
			URL:   "https://example.com",
			Title: "Example Domain",
			Body:  "This domain is for use in illustrative examples in documents.",
		},
	}

	for _, pg := range seeds {
		b, _ := json.Marshal(pg)
		if err := p.Send(context.Background(), kafka.Message{
			Key:   []byte(pg.ID),
			Value: b,
		}); err != nil {
			log.Fatal(err)
		}
		log.Printf("sent page %s", pg.ID)
	}

	time.Sleep(1 * time.Second)
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
