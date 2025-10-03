package kafka

import "context"

// Client is a tiny interface placeholder to abstract Kafka operations.
type Client interface {
    Produce(ctx context.Context, topic string, key, value []byte) error
    Consume(ctx context.Context, topic string, group string, handle func(key, value []byte) error) error
}

// NoopClient provides a stub implementation for local development.
type NoopClient struct{}

func (NoopClient) Produce(ctx context.Context, topic string, key, value []byte) error { return nil }
func (NoopClient) Consume(ctx context.Context, topic string, group string, handle func(key, value []byte) error) error {
    return nil
}

