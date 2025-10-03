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
