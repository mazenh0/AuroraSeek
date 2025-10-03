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
