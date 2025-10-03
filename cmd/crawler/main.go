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
	brokers := []string{os.Getenv("KAFKA_BROKERS")}
	topic := getenv("KAFKA_TOPIC_PAGES", "pages")

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
