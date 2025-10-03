APP?=auroraseek

.PHONY: all build tidy proto run-crawler run-indexer run-query reranker

all: build

build:
	go build ./...

tidy:
	go mod tidy

proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/search.proto || echo "Install protoc and protoc-gen-go, protoc-gen-go-grpc to generate stubs"

run-crawler:
	go run ./cmd/crawler

run-indexer:
	go run ./cmd/indexer

run-query:
	go run ./cmd/query

reranker:
	uvicorn reranker.app:app --host 0.0.0.0 --port 8000 --reload

