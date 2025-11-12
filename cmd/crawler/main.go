package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mazenh0/auroraseek/internal/crawler"
	"github.com/mazenh0/auroraseek/internal/kafka"
)

type Page struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

func main() {
	// Get configuration from environment
	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	if kafkaBrokers == "" {
		kafkaBrokers = "localhost:9092"
		log.Printf("WARNING: KAFKA_BROKERS not set, using default: %s", kafkaBrokers)
	} else {
		log.Printf("Using KAFKA_BROKERS: %s", kafkaBrokers)
	}

	brokers := []string{kafkaBrokers}
	topic := getenv("CRAWLER_TOPIC", "pages")

	// Get seed URLs from environment or use defaults
	seedURLs := getSeedURLs()
	maxPages := getIntEnv("CRAWLER_MAX_PAGES", 100)
	maxConcurrency := getIntEnv("CRAWLER_MAX_CONCURRENCY", 3)

	log.Printf("Starting crawler with seed URLs: %v", seedURLs)
	log.Printf("Max pages: %d, Max concurrency: %d", maxPages, maxConcurrency)
	log.Printf("Connecting to Kafka at %v, topic: %s", brokers, topic)

	// Initialize Kafka producer
	producer := kafka.NewProducer(brokers, topic)
	defer producer.Close()

	// Initialize crawler
	c := crawler.NewCrawler()
	crawlQueue := crawler.NewCrawlQueue()

	// Add seed URLs to queue
	crawlQueue.Add(seedURLs...)

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Received shutdown signal, stopping crawler...")
		cancel()
	}()

	// Worker pool for concurrent crawling
	pagesCrawled := 0
	var pagesMu sync.Mutex
	workerDone := make(chan bool, maxConcurrency)

	// Start workers
	for i := 0; i < maxConcurrency; i++ {
		go func(workerID int) {
			defer func() { workerDone <- true }()
			
			for {
				// Check if we should stop
				select {
				case <-ctx.Done():
					return
				default:
				}

				// Check page limit
				pagesMu.Lock()
				if pagesCrawled >= maxPages {
					pagesMu.Unlock()
					return
				}
				pagesMu.Unlock()

				// Get next URL from queue
				url, ok := crawlQueue.Next()
				if !ok {
					// Queue empty, wait a bit
					select {
					case <-ctx.Done():
						return
					case <-time.After(2 * time.Second):
						continue
					}
				}

				// Fetch page
				log.Printf("[Worker %d] Fetching: %s", workerID, url)
				page, err := c.Fetch(ctx, url)
				if err != nil {
					log.Printf("[Worker %d] Error fetching %s: %v", workerID, url, err)
					continue
				}

				// Generate ID from URL (MD5 hash)
				id := generateID(page.URL)

				// Create page message
				pg := Page{
					ID:    id,
					URL:   page.URL,
					Title: page.Title,
					Body:  page.Body,
				}

				// Send to Kafka
				b, err := json.Marshal(pg)
				if err != nil {
					log.Printf("[Worker %d] Error marshaling page: %v", workerID, err)
					continue
				}

				if err := producer.Send(ctx, kafka.Message{
					Key:   []byte(pg.ID),
					Value: b,
				}); err != nil {
					log.Printf("[Worker %d] Error sending to Kafka: %v", workerID, err)
					continue
				}

				log.Printf("[Worker %d] Indexed page: %s (%s)", workerID, pg.ID, pg.URL)

				// Update counter
				pagesMu.Lock()
				pagesCrawled++
				currentCount := pagesCrawled
				pagesMu.Unlock()

				// Add links to queue (limit to prevent explosion)
				if len(page.Links) > 0 && currentCount < maxPages {
					// Take up to 10 links per page
					linksToAdd := page.Links
					if len(linksToAdd) > 10 {
						linksToAdd = linksToAdd[:10]
					}
					crawlQueue.Add(linksToAdd...)
					log.Printf("[Worker %d] Added %d links to queue (queue size: %d)", 
						workerID, len(linksToAdd), crawlQueue.Size())
				}

				// Check page limit again
				if currentCount >= maxPages {
					log.Printf("[Worker %d] Reached max pages limit (%d), stopping", workerID, maxPages)
					return
				}
			}
		}(i)
	}

	// Wait for all workers to finish
	for i := 0; i < maxConcurrency; i++ {
		<-workerDone
	}

	log.Printf("Crawler finished. Total pages crawled: %d", pagesCrawled)
}

// generateID generates a unique ID from a URL using MD5
func generateID(url string) string {
	hash := md5.Sum([]byte(url))
	return hex.EncodeToString(hash[:])
}

// getSeedURLs gets seed URLs from environment or returns defaults
func getSeedURLs() []string {
	seedsEnv := os.Getenv("CRAWLER_SEED_URLS")
	if seedsEnv != "" {
		// Split by comma
		urls := strings.Split(seedsEnv, ",")
		// Trim whitespace
		for i, u := range urls {
			urls[i] = strings.TrimSpace(u)
		}
		return urls
	}
	// Default seed URLs
	return []string{
		"https://example.com",
		"https://golang.org",
	}
}

// getIntEnv gets an integer from environment variable or returns default
func getIntEnv(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	var result int
	if _, err := fmt.Sscanf(value, "%d", &result); err != nil {
		return defaultValue
	}
	return result
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
