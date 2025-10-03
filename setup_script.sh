#!/bin/bash

set -e

echo "🚀 Setting up AuroraSeek project..."
echo ""

# Create directory structure
echo "📁 Creating directory structure..."
mkdir -p cmd/crawler cmd/indexer cmd/query
mkdir -p internal/bm25 internal/kafka internal/index internal/util
mkdir -p proto reranker k8s gen

# Create .gitignore
echo "📝 Creating .gitignore..."
cat > .gitignore << 'EOF'
# Binaries for programs and plugins
*.exe
*.exe~
*.dll
*.so
*.dylib

# Test binary, built with `go test -c`
*.test

# Output of the go coverage tool
*.out

# Go workspace file
go.work

# Dependency directories
vendor/

# Go build cache
.cache/

# IDE specific files
.vscode/
.idea/
*.swp
*.swo
*~

# OS specific files
.DS_Store
Thumbs.db

# Generated protobuf files
gen/

# Python
__pycache__/
*.py[cod]
*$py.class
*.so
.Python
venv/
env/
ENV/
.venv

# Docker
*.log

# Environment variables
.env
EOF

# Create .env.example
echo "📝 Creating .env.example..."
cat > .env.example << 'EOF'
# Kafka / Redpanda
KAFKA_BROKERS=redpanda:9092
KAFKA_TOPIC_PAGES=pages
KAFKA_TOPIC_DOCS=docs

# Query service
QUERY_ADDR=:50051
RERANKER_URL=http://reranker:8000/rerank

# Indexer
INDEXER_SHARD_ID=0
EOF

# Create go.mod
echo "📝 Creating go.mod..."
cat > go.mod << 'EOF'
module github.com/mazenh0/auroraseek

go 1.22

require (
	github.com/segmentio/kafka-go v0.4.47
	google.golang.org/grpc v1.65.0
	google.golang.org/protobuf v1.34.1
)
EOF

# Create Makefile
echo "📝 Creating Makefile..."
cat > Makefile << 'EOF'
.PHONY: proto run up down tidy

proto:
	protoc --go_out=. --go-grpc_out=. proto/search.proto

tidy:
	go mod tidy

up:
	docker compose up -d --build

down:
	docker compose down -v

run:
	go run ./cmd/query &
	go run ./cmd/indexer &
	go run ./cmd/crawler &
	python reranker/app.py
EOF

# Create Dockerfile
echo "📝 Creating Dockerfile..."
cat > Dockerfile << 'EOF'
# syntax=docker/dockerfile:1
FROM golang:1.22 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/crawler ./cmd/crawler && \
    CGO_ENABLED=0 GOOS=linux go build -o /app/indexer ./cmd/indexer && \
    CGO_ENABLED=0 GOOS=linux go build -o /app/query ./cmd/query

FROM gcr.io/distroless/base-debian12
COPY --from=build /app /app
ENTRYPOINT ["/bin/sh", "-c", "echo Specify command in docker-compose"]
EOF

# Create docker-compose.yml
echo "📝 Creating docker-compose.yml..."
cat > docker-compose.yml << 'EOF'
version: "3.9"

services:
  redpanda:
    image: docker.redpanda.com/redpandadata/redpanda:v24.2.7
    command: 
      - redpanda
      - start
      - --overprovisioned
      - --smp
      - "1"
      - --memory
      - 1G
      - --reserve-memory
      - 0M
      - --node-id
      - "0"
      - --check=false
    ports:
      - "9092:9092"

  reranker:
    build: ./reranker
    command: ["python", "app.py"]
    ports:
      - "8000:8000"

  indexer:
    build: .
    command: ["/app/indexer"]
    environment:
      - KAFKA_BROKERS=redpanda:9092
      - KAFKA_TOPIC_PAGES=pages
    depends_on:
      - redpanda

  crawler:
    build: .
    command: ["/app/crawler"]
    environment:
      - KAFKA_BROKERS=redpanda:9092
      - KAFKA_TOPIC_PAGES=pages
    depends_on:
      - redpanda

  query:
    build: .
    command: ["/app/query"]
    environment:
      - QUERY_ADDR=:50051
      - RERANKER_URL=http://reranker:8000/rerank
    ports:
      - "50051:50051"
    depends_on:
      - indexer
      - reranker
EOF

# Create proto/search.proto
echo "📝 Creating proto/search.proto..."
cat > proto/search.proto << 'EOF'
syntax = "proto3";
package search;

option go_package = "github.com/mazenh0/auroraseek/gen/searchpb";

message Document {
  string id = 1;
  string url = 2;
  string title = 3;
  string body = 4;
}

message QueryRequest {
  string query = 1;
  int32 k = 2; // top-k
}

message ScoredDocument {
  Document doc = 1;
  double score = 2;
}

message QueryResponse {
  repeated ScoredDocument results = 1;
}

service SearchService {
  rpc Search (QueryRequest) returns (QueryResponse);
}
EOF

# Create internal/util/text.go
echo "📝 Creating internal/util/text.go..."
cat > internal/util/text.go << 'EOF'
package util

import (
	"regexp"
	"strings"
)

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

func Normalize(s string) string {
	s = strings.ToLower(s)
	s = nonAlphaNum.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func Tokens(s string) []string {
	s = Normalize(s)
	if s == "" {
		return nil
	}
	return strings.Fields(s)
}
EOF

# Create internal/bm25/bm25.go
echo "📝 Creating internal/bm25/bm25.go..."
cat > internal/bm25/bm25.go << 'EOF'
package bm25

import (
	"math"
)

type BM25 struct {
	K1    float64
	B     float64
	AvgDL float64
	N     int
	DF    map[string]int // doc frequency per term
}

func New(N int, avgDL float64, df map[string]int) *BM25 {
	return &BM25{K1: 1.2, B: 0.75, AvgDL: avgDL, N: N, DF: df}
}

func (b *BM25) IDF(term string) float64 {
	df := b.DF[term]
	if df == 0 {
		df = 1
	}
	return math.Log((float64(b.N)-float64(df)+0.5)/(float64(df)+0.5) + 1)
}

func (b *BM25) Score(tf map[string]int, dl int, terms []string) float64 {
	var score float64
	for _, t := range terms {
		f := float64(tf[t])
		if f == 0 {
			continue
		}
		idf := b.IDF(t)
		denom := f + b.K1*(1-b.B+b.B*float64(dl)/b.AvgDL)
		score += idf * (f * (b.K1 + 1)) / denom
	}
	return score
}
EOF

# Create internal/index/memindex.go
echo "📝 Creating internal/index/memindex.go..."
cat > internal/index/memindex.go << 'EOF'
package index

import (
	"sync"

	"github.com/mazenh0/auroraseek/internal/util"
)

type Doc struct {
	ID    string
	URL   string
	Title string
	Body  string
	TF    map[string]int
	DL    int
}

type MemIndex struct {
	mu       sync.RWMutex
	postings map[string]map[string]int // term -> docID -> tf
	docs     map[string]*Doc
	df       map[string]int
	totalLen int
}

func NewMem() *MemIndex {
	return &MemIndex{
		postings: map[string]map[string]int{},
		docs:     map[string]*Doc{},
		df:       map[string]int{},
	}
}

func (m *MemIndex) Add(id, url, title, body string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	toks := util.Tokens(title + " " + body)
	tf := map[string]int{}
	for _, t := range toks {
		tf[t]++
	}

	d := &Doc{ID: id, URL: url, Title: title, Body: body, TF: tf, DL: len(toks)}
	m.docs[id] = d
	m.totalLen += d.DL

	seen := map[string]bool{}
	for term, f := range tf {
		if m.postings[term] == nil {
			m.postings[term] = map[string]int{}
		}
		m.postings[term][id] = f
		if !seen[term] {
			m.df[term]++
			seen[term] = true
		}
	}
}

func (m *MemIndex) Snapshot() (N int, avgDL float64, df map[string]int) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	N = len(m.docs)
	if N == 0 {
		return 0, 1, map[string]int{}
	}
	avgDL = float64(m.totalLen) / float64(N)

	// shallow copy of df
	df = map[string]int{}
	for k, v := range m.df {
		df[k] = v
	}
	return
}

func (m *MemIndex) Candidates(terms []string) map[string]*Doc {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cand := map[string]*Doc{}
	for _, t := range terms {
		for docID := range m.postings[t] {
			cand[docID] = m.docs[docID]
		}
	}
	return cand
}
EOF

# Create internal/kafka/kafka.go
echo "📝 Creating internal/kafka/kafka.go..."
cat > internal/kafka/kafka.go << 'EOF'
package kafka

import (
	"context"
	"time"

	k "github.com/segmentio/kafka-go"
)

type Producer struct {
	w *k.Writer
}

type Message struct {
	Key, Value []byte
}

func NewProducer(brokers []string, topic string) *Producer {
	return &Producer{
		w: &k.Writer{
			Addr:     k.TCP(brokers...),
			Topic:    topic,
			Balancer: &k.LeastBytes{},
		},
	}
}

func (p *Producer) Close() error {
	return p.w.Close()
}

func (p *Producer) Send(ctx context.Context, msgs ...Message) error {
	km := make([]k.Message, len(msgs))
	for i, m := range msgs {
		km[i] = k.Message{Key: m.Key, Value: m.Value}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return p.w.WriteMessages(ctx, km...)
}

// Consumer helper
func NewReader(brokers []string, topic, group string) *k.Reader {
	return k.NewReader(k.ReaderConfig{
		Brokers:     brokers,
		Topic:       topic,
		GroupID:     group,
		StartOffset: k.FirstOffset,
	})
}
EOF

# Create cmd/crawler/main.go
echo "📝 Creating cmd/crawler/main.go..."
cat > cmd/crawler/main.go << 'EOF'
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
EOF

# Create cmd/indexer/main.go
echo "📝 Creating cmd/indexer/main.go..."
cat > cmd/indexer/main.go << 'EOF'
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
	brokers := []string{os.Getenv("KAFKA_BROKERS")}
	topic := getenv("KAFKA_TOPIC_PAGES", "pages")
	group := "indexer"

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
			log.Fatal(err)
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
EOF

# Create cmd/query/main.go
echo "📝 Creating cmd/query/main.go..."
cat > cmd/query/main.go << 'EOF'
package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"

	pb "github.com/mazenh0/auroraseek/gen/searchpb"
	"github.com/mazenh0/auroraseek/internal/bm25"
	"github.com/mazenh0/auroraseek/internal/index"
	"github.com/mazenh0/auroraseek/internal/util"
	"google.golang.org/grpc"
)

// For demo, share in-memory index in-process.
var mem = index.NewMem()

// Seed a couple docs so Search works even before indexer loads.
func seed() {
	mem.Add("1", "https://example.com", "Example Domain", "This domain is for use in illustrative examples in documents.")
	mem.Add("2", "https://golang.org", "Go", "Go is an open source programming language that makes it easy to build simple, reliable, and efficient software.")
}

type server struct {
	pb.UnimplementedSearchServiceServer
}

func (s *server) Search(ctx context.Context, req *pb.QueryRequest) (*pb.QueryResponse, error) {
	terms := util.Tokens(req.Query)
	cand := mem.Candidates(terms)

	N, avgDL, df := mem.Snapshot()
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func main() {
	seed()

	lis, err := net.Listen("tcp", getenv("QUERY_ADDR", ":50051"))
	if err != nil {
		log.Fatal(err)
	}

	s := grpc.NewServer()
	pb.RegisterSearchServiceServer(s, &server{})
	log.Println("query service listening on", lis.Addr())

	if err := s.Serve(lis); err != nil {
		log.Fatal(err)
	}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
EOF

# Create reranker/app.py
echo "📝 Creating reranker/app.py..."
cat > reranker/app.py << 'EOF'
from fastapi import FastAPI
from pydantic import BaseModel
from typing import List, Dict
import uvicorn
import math

app = FastAPI()

class Candidate(BaseModel):
    id: str
    title: str
    body: str

class RerankIn(BaseModel):
    query: str
    candidates: List[Candidate]

class RerankOut(BaseModel):
    order: List[str]

# Tiny bag-of-words "embedding"
def embed(text: str) -> Dict[str, float]:
    v = {}
    for tok in text.lower().split():
        v[tok] = v.get(tok, 0.0) + 1.0
    # l2 normalize
    norm = math.sqrt(sum(t*t for t in v.values())) or 1.0
    for k in v:
        v[k] /= norm
    return v

def cosine(a: Dict[str,float], b: Dict[str,float]) -> float:
    keys = a.keys() & b.keys()
    return sum(a[k]*b[k] for k in keys)

@app.post("/rerank", response_model=RerankOut)
async def rerank(inp: RerankIn):
    qv = embed(inp.query)
    scored = []
    for c in inp.candidates:
        dv = embed(c.title + " " + c.body)
        scored.append((c.id, cosine(qv, dv)))
    scored.sort(key=lambda x: x[1], reverse=True)
    ordered = [sid for sid,_ in scored]
    return RerankOut(order=ordered)

if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8000)
EOF

# Create reranker/Dockerfile
echo "📝 Creating reranker/Dockerfile..."
cat > reranker/Dockerfile << 'EOF'
FROM python:3.11-slim

WORKDIR /app
COPY app.py ./
RUN pip install fastapi==0.115.0 uvicorn==0.30.6

EXPOSE 8000
CMD ["python", "app.py"]
EOF

# Create README.md
echo "📝 Creating README.md..."
cat > README.md << 'EOF'
# AuroraSeek - Minimal Distributed Search Engine

A lean, runnable starter for a distributed search stack with BM25 + vectors, Kafka pipelines, gRPC, and a Python reranker.

## Features

- **BM25 Ranking**: Classic information retrieval algorithm for text search
- **Kafka/Redpanda**: Message queue for distributed indexing pipeline
- **gRPC**: High-performance RPC for search queries
- **Python Reranker**: FastAPI-based reranking service using cosine similarity
- **In-memory Index**: Fast inverted index for demo purposes

## Architecture

```
┌──────────┐     ┌──────────┐     ┌──────────┐
│ Crawler  │────▶│  Kafka   │────▶│ Indexer  │
└──────────┘     │(Redpanda)│     └──────────┘
                 └──────────┘            │
                                         ▼
┌──────────┐                      ┌──────────┐
│  Query   │◀────────────────────▶│  Index   │
│ Service  │                      │(In-Mem)  │
└──────────┘                      └──────────┘
     │
     ▼
┌──────────┐
│Reranker  │
│(Python)  │
└──────────┘
```

## Prerequisites

- **Go 1.22+**
- **Python 3.11+**
- **Docker & Docker Compose**
- **protoc** (Protocol Buffer compiler)

### Install protoc

```bash
# macOS
brew install protobuf

# Ubuntu/Debian
apt install -y protobuf-compiler

# Or download from: https://github.com/protocolbuffers/protobuf/releases
```

### Install Go protoc plugins

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Make sure $GOPATH/bin is in your PATH
export PATH="$PATH:$(go env GOPATH)/bin"
```

## Quick Start

### 1. Setup

```bash
# Initialize Go modules and install dependencies
go mod tidy
```

### 2. Generate Protobuf Code

```bash
make proto
```

This generates Go code from `proto/search.proto` into the `gen/searchpb/` directory.

### 3. Build and Run with Docker Compose

```bash
make up
```

This will:
- Start Redpanda (Kafka)
- Build and start the Reranker service
- Build and start the Indexer service
- Build and start the Crawler service
- Build and start the Query service

### 4. Test the Search

Install grpcurl for testing:

```bash
# macOS
brew install grpcurl

# Go install
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
```

Run a search query:

```bash
grpcurl -plaintext \
  -d '{"query":"example domain", "k": 5}' \
  localhost:50051 \
  search.SearchService/Search
```

Expected output:

```json
{
  "results": [
    {
      "doc": {
        "id": "1",
        "url": "https://example.com",
        "title": "Example Domain",
        "body": "This domain is for use in illustrative examples in documents."
      },
      "score": 1.234
    }
  ]
}
```

## Development

### Run Locally (without Docker)

```bash
make run
```

### Stop Services

```bash
make down
```

## Project Structure

```
auroraseek/
├── cmd/
│   ├── crawler/     # Crawler service (Kafka producer)
│   ├── indexer/     # Indexer service (Kafka consumer)
│   └── query/       # Query service (gRPC server)
├── internal/
│   ├── bm25/        # BM25 ranking algorithm
│   ├── kafka/       # Kafka producer/consumer helpers
│   ├── index/       # In-memory inverted index
│   └── util/        # Text normalization utilities
├── proto/           # Protocol Buffer definitions
├── reranker/        # Python FastAPI reranker service
├── k8s/             # Kubernetes deployment files
└── gen/             # Generated protobuf code
```

## Notes

- Index is in-memory; persistence, sharding, and ANN are out of scope for v0.1
- Reranker is a trivial cosine baseline; drop in a cross-encoder later
- Swap Redpanda with managed Kafka in prod; add mTLS/JWT at the gateway

## License

MIT
EOF

echo ""
echo "✅ All files created successfully!"
echo ""
echo "📋 Next steps:"
echo "1. Run: go mod tidy"
echo "2. Install protoc and Go plugins (see README)"
echo "3. Run: make proto"
echo "4. Run: make up"
echo ""
echo "🎉 Your AuroraSeek project is ready!"
EOF

chmod +x setup.sh
