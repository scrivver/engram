package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/chunhou/engram/internal/model"
	amqp "github.com/rabbitmq/amqp091-go"
)

const QueueName = "engram.ingest"

type Publisher struct {
	conn *amqp.Connection
	ch   *amqp.Channel
}

func NewPublisher(amqpURL string) (*Publisher, error) {
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, fmt.Errorf("connect to rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("open channel: %w", err)
	}

	_, err = ch.QueueDeclare(QueueName, true, false, false, false, nil)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("declare queue: %w", err)
	}

	log.Printf("RabbitMQ publisher ready (queue=%s)", QueueName)
	return &Publisher{conn: conn, ch: ch}, nil
}

func (p *Publisher) PublishIngestJob(ctx context.Context, job model.IngestJob) error {
	body, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}

	return p.ch.PublishWithContext(ctx, "", QueueName, false, false, amqp.Publishing{
		DeliveryMode: amqp.Persistent,
		ContentType:  "application/json",
		Body:         body,
	})
}

func (p *Publisher) Close() {
	p.ch.Close()
	p.conn.Close()
}
