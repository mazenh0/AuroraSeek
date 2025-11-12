package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	pb "github.com/mazenh0/auroraseek/gen/searchpb"
	"github.com/mazenh0/auroraseek/internal/bm25"
	"github.com/mazenh0/auroraseek/internal/index"
	"github.com/mazenh0/auroraseek/internal/util"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc"
)

var (
	mem        *index.PersistentIndex
	memMu      sync.RWMutex
	lastReload time.Time
)

// loadIndex loads the persistent index from disk
func loadIndex() error {
	dbPath := getenv("INDEX_DB_PATH", "/data/index.db")
	log.Printf("Loading index from: %s", dbPath)

	var err error
	mem, err = index.NewPersistent(dbPath)
	if err != nil {
		return fmt.Errorf("failed to load persistent index: %w", err)
	}

	docCount, totalLen, _ := mem.Stats()
	log.Printf("Loaded index with %d documents, %d total tokens", docCount, totalLen)
	lastReload = time.Now()

	// If index is empty, add sample documents (only on first load)
	if docCount == 0 {
		log.Println("Index is empty, adding sample documents...")
		mem.Add("1", "https://example.com", "Example Domain", "This domain is for use in illustrative examples in documents.")
		mem.Add("2", "https://golang.org", "Go", "Go is an open source programming language that makes it easy to build simple, reliable, and efficient software.")
	}

	return nil
}

// reloadIndex reloads the index from disk to pick up new documents
// It only reloads if the document count has increased
func reloadIndex() error {
	memMu.RLock()
	oldMem := mem
	if oldMem == nil {
		memMu.RUnlock()
		return fmt.Errorf("index not initialized")
	}
	oldDocCount, _, _ := oldMem.Stats()
	memMu.RUnlock()

	// Quick check: get document count from database without full reload
	dbPath := getenv("INDEX_DB_PATH", "/data/index.db")
	
	// Open database in read-only mode to check document count
	db, err := bbolt.Open(dbPath, 0666, &bbolt.Options{ReadOnly: true})
	if err != nil {
		// If we can't open (e.g., database is locked), skip reload
		return nil
	}

	var newDocCount int
	err = db.View(func(tx *bbolt.Tx) error {
		docsBucket := tx.Bucket([]byte("docs"))
		if docsBucket == nil {
			return nil
		}
		stats := docsBucket.Stats()
		newDocCount = stats.KeyN
		return nil
	})
	db.Close()

	if err != nil {
		return nil // Skip reload on error
	}

	// Only reload if document count increased
	if newDocCount > oldDocCount {
		log.Printf("Detected new documents: %d -> %d (+%d), reloading index...", oldDocCount, newDocCount, newDocCount-oldDocCount)
		
		// Create new persistent index
		newMem, err := index.NewPersistent(dbPath)
		if err != nil {
			return fmt.Errorf("failed to reload index: %w", err)
		}

		// Atomically replace the index
		memMu.Lock()
		if oldMem != nil {
			oldMem.Close()
		}
		mem = newMem
		lastReload = time.Now()
		memMu.Unlock()
		
		log.Printf("Successfully reloaded index: %d documents", newDocCount)
	}

	return nil
}

type server struct {
	pb.UnimplementedSearchServiceServer
}

func (s *server) Search(ctx context.Context, req *pb.QueryRequest) (*pb.QueryResponse, error) {
	// Get a read lock on the index
	memMu.RLock()
	currentMem := mem
	memMu.RUnlock()

	if currentMem == nil {
		return nil, fmt.Errorf("index not initialized")
	}

	terms := util.Tokens(req.Query)
	cand := currentMem.Candidates(terms)

	N, avgDL, df := currentMem.Snapshot()
	bm := bm25.New(N, avgDL, df)

	type scored struct {
		d     *index.Doc
		score float64
	}
	var list []scored

	for _, d := range cand {
		list = append(list, scored{d, bm.Score(d.TF, d.DL, terms)})
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].score > list[j].score
	})

	// Prepare candidates for reranker
	top := min(len(list), max(20, int(req.K)))
	cands := make([]map[string]string, 0, top)
	for i := 0; i < top; i++ {
		cands = append(cands, map[string]string{
			"id":    list[i].d.ID,
			"title": list[i].d.Title,
			"body":  list[i].d.Body,
		})
	}

	reranked := callReranker(os.Getenv("RERANKER_URL"), req.Query, cands)

	// Merge reranked order
	id2score := map[string]float64{}
	for i, id := range reranked {
		id2score[id] = float64(len(reranked) - i)
	}

	sort.Slice(list, func(i, j int) bool {
		si := id2score[list[i].d.ID]
		sj := id2score[list[j].d.ID]
		if si == sj {
			return list[i].score > list[j].score
		}
		return si > sj
	})

	k := min(len(list), int(req.K))
	res := &pb.QueryResponse{}
	for i := 0; i < k; i++ {
		d := list[i].d
		res.Results = append(res.Results, &pb.ScoredDocument{
			Doc: &pb.Document{
				Id:    d.ID,
				Url:   d.URL,
				Title: d.Title,
				Body:  truncate(d.Body, 512),
			},
			Score: list[i].score,
		})
	}

	return res, nil
}

func callReranker(url, query string, cands []map[string]string) []string {
	if url == "" {
		return ids(cands)
	}

	body := map[string]any{"query": query, "candidates": cands}
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", strings.NewReader(string(b)))
	if err != nil {
		return ids(cands)
	}
	defer resp.Body.Close()

	var out struct {
		Order []string `json:"order"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ids(cands)
	}
	if len(out.Order) == 0 {
		return ids(cands)
	}
	return out.Order
}

func ids(c []map[string]string) []string {
	r := make([]string, len(c))
	for i, x := range c {
		r[i] = x["id"]
	}
	return r
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func main() {
	// Load persistent index
	if err := loadIndex(); err != nil {
		log.Fatalf("Failed to load index: %v", err)
	}
	defer mem.Close()

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

	// Periodic index reload to pick up new documents
	go func() {
		ticker := time.NewTicker(30 * time.Second) // Reload every 30 seconds
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := reloadIndex(); err != nil {
					log.Printf("Error reloading index: %v", err)
				}
			}
		}
	}()

	// Health check endpoint
	go func() {
		http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			memMu.RLock()
			currentMem := mem
			reloadTime := lastReload
			memMu.RUnlock()

			docCount := 0
			if currentMem != nil {
				docCount, _, _ = currentMem.Stats()
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":      "ok",
				"doc_count":   docCount,
				"index_path":  getenv("INDEX_DB_PATH", "/data/index.db"),
				"last_reload": reloadTime.Format(time.RFC3339),
			})
		})
		log.Println("Health check endpoint listening on :8080")
		log.Fatal(http.ListenAndServe(":8080", nil))
	}()

	// Start gRPC server
	lis, err := net.Listen("tcp", getenv("QUERY_ADDR", ":50051"))
	if err != nil {
		log.Fatal(err)
	}

	s := grpc.NewServer()
	pb.RegisterSearchServiceServer(s, &server{})
	log.Println("Query service listening on", lis.Addr())

	// Start server in a goroutine
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	log.Println("Shutting down query service...")
	s.GracefulStop()
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
