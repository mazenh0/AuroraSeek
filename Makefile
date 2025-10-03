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
