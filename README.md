# 🔍 AuroraSeek

> **A minimal, production-ready distributed search engine** that actually works.

Build your own Google. Well, maybe not quite, but this is a real search engine that crawls the web, indexes content, and serves semantic search results using BM25 + modern embeddings. Perfect for learning, prototyping, or building something awesome.

## ✨ What's Inside

- 🕷️ **Real Web Crawler** - Actually fetches and parses web pages (no mock data!)
- 🎯 **BM25 Ranking** - Classic but powerful keyword-based search
- 🧠 **Semantic Embeddings** - Uses sentence-transformers for understanding meaning
- 🚀 **Distributed Architecture** - Kafka-based pipeline that scales
- ⚡ **Fast gRPC API** - High-performance search endpoint
- 🐍 **Python Reranker** - Semantic reranking service

## 🏗️ How It Works

```
┌──────────┐     ┌──────────┐     ┌──────────┐
│  Crawler │────▶│  Kafka   │────▶│ Indexer  │
│  🕷️      │     │ 📦       │     │ 📚       │
└──────────┘     └──────────┘     └──────────┘
                       │                │
                       │                ▼
┌──────────┐           │         ┌──────────┐
│  Query   │◀──────────┼────────▶│  Index   │
│  🔍      │           │         │ 💾       │
└──────────┘           │         └──────────┘
     │                 │
     ▼                 │
┌──────────┐           │
│ Reranker │◀──────────┘
│ 🧠       │
└──────────┘
```

1. **Crawler** finds web pages and sends them to Kafka
2. **Indexer** consumes from Kafka and builds an inverted index
3. **Query Service** receives search requests via gRPC
4. **BM25** ranks documents by keyword relevance
5. **Reranker** uses semantic embeddings to improve results
6. **You** get awesome search results! 🎉

## 🚀 Quick Start

### Prerequisites

Make sure you have these installed:
- **Go 1.22+** - [Download](https://go.dev/dl/)
- **Python 3.11+** - [Download](https://www.python.org/downloads/)
- **Docker & Docker Compose** - [Download](https://www.docker.com/get-started)
- **protoc** - Protocol Buffer compiler

#### Install protoc

```bash
# macOS
brew install protobuf

# Ubuntu/Debian
apt install -y protobuf-compiler

# Or download from: https://github.com/protocolbuffers/protobuf/releases
```

#### Install Go protoc plugins

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Make sure $GOPATH/bin is in your PATH
export PATH="$PATH:$(go env GOPATH)/bin"
```

### Let's Go! 🎯

```bash
# 1. Install dependencies
go mod tidy

# 2. Generate protobuf code
make proto

# 3. Start everything with Docker Compose
make up

# 4. Wait a few seconds for services to start, then search!
```

### Test It Out

Install `grpcurl` (if you don't have it):

```bash
# macOS
brew install grpcurl

# Or via Go
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
```

Run a search query:

```bash
grpcurl -plaintext \
  -d '{"query":"programming language", "k": 5}' \
  localhost:50051 \
  search.SearchService/Search
```

You should see results like:

```json
{
  "results": [
    {
      "doc": {
        "id": "...",
        "url": "https://golang.org",
        "title": "Go",
        "body": "Go is an open source programming language..."
      },
      "score": 2.34
    }
  ]
}
```

## ⚙️ Configuration

### Crawler Settings

Customize the crawler via environment variables:

```bash
# Seed URLs to start crawling from
CRAWLER_SEED_URLS="https://example.com,https://golang.org"

# Maximum number of pages to crawl
CRAWLER_MAX_PAGES=100

# Number of concurrent workers
CRAWLER_MAX_CONCURRENCY=3

# Kafka topic for pages
CRAWLER_TOPIC=pages
```

Example with Docker:

```bash
docker run -e CRAWLER_SEED_URLS="https://example.com" \
           -e CRAWLER_MAX_PAGES=500 \
           -e CRAWLER_MAX_CONCURRENCY=5 \
           auroraseek-crawler
```

## 🛠️ Development

### Run Locally (without Docker)

```bash
make run
```

This starts all services locally. Make sure you have:
- Redpanda/Kafka running on `localhost:9092`
- Python dependencies installed: `pip install -r reranker/requirements.txt`

### Stop Services

```bash
make down
```

### Project Structure

```
auroraseek/
├── cmd/
│   ├── crawler/     # Web crawler service
│   ├── indexer/     # Index builder service
│   └── query/       # Search API service
├── internal/
│   ├── bm25/        # BM25 ranking algorithm
│   ├── crawler/     # Web crawling utilities
│   ├── index/       # In-memory inverted index
│   ├── kafka/       # Kafka producer/consumer
│   └── util/        # Text processing utilities
├── proto/           # Protocol Buffer definitions
├── reranker/        # Python semantic reranker
├── k8s/             # Kubernetes deployments
└── gen/             # Generated protobuf code
```

## 🎓 What Makes This Special

### 🕷️ Smart Web Crawler

- **Real HTTP fetching** with proper error handling
- **HTML parsing** using goquery (like jQuery for Go!)
- **URL normalization** to avoid duplicates
- **Rate limiting** per domain (be nice to servers!)
- **Link following** to discover new pages
- **Concurrent workers** for parallel crawling

### 🧠 Semantic Search

The reranker uses **sentence-transformers** (`all-MiniLM-L6-v2`) for semantic understanding:
- **Understands meaning**, not just keywords
- **Handles synonyms** and paraphrasing
- **Context-aware** ranking
- **Fast inference** with a lightweight model

This means you can search for "programming language" and find results about "Go", "Python", etc., even if those exact words aren't in the query!

### ⚡ Performance

- **BM25** for fast keyword matching
- **Semantic embeddings** for relevance
- **gRPC** for low-latency API
- **Kafka** for scalable message processing
- **In-memory index** for instant queries (for demo purposes)

## 📝 Notes & Limitations

- **Index is in-memory** - Data is lost on restart (persistence coming soon!)
- **No sharding** - Single index instance (fine for demos)
- **No vector database** - Embeddings computed on-the-fly (FAISS/Qdrant coming soon)
- **Basic robots.txt** - Respects some rules but not full compliance yet

## 🔮 What's Next?

Potential improvements:
- [ ] Persistent index storage (bbolt/BadgerDB)
- [ ] Vector database integration (FAISS/Qdrant)
- [ ] Distributed sharding
- [ ] Full robots.txt support
- [ ] Authentication & authorization
- [ ] Query analytics
- [ ] Multi-language support

## 🤝 Contributing

This is a learning project, but contributions are welcome! Feel free to:
- Open issues for bugs or feature requests
- Submit pull requests
- Share your thoughts and ideas

## 📄 License

MIT License - feel free to use this however you want!

## 🙏 Acknowledgments

Built with:
- [Go](https://go.dev/) - The language of choice
- [sentence-transformers](https://www.sbert.net/) - For semantic embeddings
- [Kafka](https://kafka.apache.org/) / [Redpanda](https://redpanda.com/) - Message queue
- [gRPC](https://grpc.io/) - High-performance RPC
- [FastAPI](https://fastapi.tiangolo.com/) - Python web framework

---

**If Google can do it, so can you. Well, maybe with a bit more code and fewer zeros in your bank account. But hey, at least you'll understand how it works! 🔍✨**
