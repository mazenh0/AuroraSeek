package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/mazenh0/auroraseek/internal/index"
	"github.com/mazenh0/auroraseek/internal/kafka"
)

type Page struct {
	ID, URL, Title, Body string
}

var mem = index.NewMem()

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
	group := "indexer"

	log.Printf("Connecting to Kafka at %v, topic: %s, group: %s", brokers, topic, group)

	r := kafka.NewReader(brokers, topic, group)
	defer r.Close()

	go func() {
		http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(200)
			w.Write([]byte("ok"))
		})
		log.Fatal(http.ListenAndServe(":8080", nil))
	}()

	for {
		m, err := r.ReadMessage(context.Background())
		if err != nil {
			log.Printf("Error reading message: %v", err)
			continue
		}

		var pg Page
		if err := json.Unmarshal(m.Value, &pg); err != nil {
			continue
		}

		mem.Add(pg.ID, pg.URL, pg.Title, pg.Body)
		log.Printf("indexed %s", pg.ID)
	}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
