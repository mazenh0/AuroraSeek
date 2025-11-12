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

	// Health check server
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
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

	healthServer := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start health check server
	go func() {
		log.Println("Health check endpoint listening on :8080")
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Health check server failed: %v", err)
		}
	}()

	// Main indexing loop
	log.Println("Starting indexer...")
	indexingDone := make(chan struct{})
	go func() {
		defer close(indexingDone)
		for {
			select {
			case <-ctx.Done():
				log.Println("Indexing loop received shutdown signal")
				return
			default:
				m, err := r.ReadMessage(ctx)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					log.Printf("Error reading message: %v", err)
					// Brief delay before retry to avoid tight loop
					select {
					case <-ctx.Done():
						return
					case <-time.After(1 * time.Second):
					}
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
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	log.Println("Shutdown signal received, starting graceful shutdown...")

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Shutdown health check server
	log.Println("Shutting down health check server...")
	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error shutting down health check server: %v", err)
	} else {
		log.Println("Health check server shut down successfully")
	}

	// Close Kafka reader
	log.Println("Closing Kafka reader...")
	if err := r.Close(); err != nil {
		log.Printf("Error closing Kafka reader: %v", err)
	} else {
		log.Println("Kafka reader closed successfully")
	}

	// Wait for indexing loop to finish (with timeout)
	log.Println("Waiting for indexing loop to finish...")
	select {
	case <-indexingDone:
		log.Println("Indexing loop finished")
	case <-shutdownCtx.Done():
		log.Println("Shutdown timeout reached, forcing shutdown")
	}

	// Close index
	log.Println("Closing index...")
	if err := mem.Close(); err != nil {
		log.Printf("Error closing index: %v", err)
	} else {
		log.Println("Index closed successfully")
	}

	log.Println("Graceful shutdown complete")
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
