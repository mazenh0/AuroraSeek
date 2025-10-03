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
