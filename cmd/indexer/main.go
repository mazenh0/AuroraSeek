package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mazenh0/auroraseek/internal/index"
	"github.com/mazenh0/auroraseek/internal/kafka"
)

type Page struct {
	ID, URL, Title, Body string
}

func main() {
	// Get database path from environment or use default
	dbPath := getenv("INDEX_DB_PATH", "/data/index.db")
	log.Printf("Opening index at: %s", dbPath)

	// Create persistent index
	mem, err := index.NewPersistent(dbPath)
	if err != nil {
		log.Fatalf("Failed to create persistent index: %v", err)
	}
	defer mem.Close()

	// Get initial stats
	docCount, totalLen, _ := mem.Stats()
	log.Printf("Loaded index with %d documents, %d total tokens", docCount, totalLen)

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

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Received shutdown signal, shutting down...")
		cancel()
	}()

	// Health check endpoint
	go func() {
		mux := http.NewServeMux()
		
		// Liveness probe
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		})
		
		// Readiness probe
		mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
			docCount, _, _ := mem.Stats()
			status := map[string]interface{}{
				"status":     "ok",
				"doc_count":  docCount,
				"index_path": dbPath,
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(status)
		})

		server := &http.Server{
			Addr:         ":8080",
			Handler:      mux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  120 * time.Second,
		}

		log.Println("Health check endpoint listening on :8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Health check server failed: %v", err)
		}
	}()

	// Main indexing loop
	log.Println("Starting indexer...")
	for {
		select {
		case <-ctx.Done():
			log.Println("Shutting down indexer...")
			return
		default:
			m, err := r.ReadMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("Error reading message: %v", err)
				continue
			}

			var pg Page
			if err := json.Unmarshal(m.Value, &pg); err != nil {
				log.Printf("Error unmarshaling page: %v", err)
				continue
			}

			if err := mem.Add(pg.ID, pg.URL, pg.Title, pg.Body); err != nil {
				log.Printf("Error adding document %s: %v", pg.ID, err)
				continue
			}

			docCount, _, _ := mem.Stats()
			log.Printf("Indexed %s (total: %d documents)", pg.ID, docCount)
		}
	}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
