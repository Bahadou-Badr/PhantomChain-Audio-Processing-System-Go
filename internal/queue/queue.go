package queue

import (
	"context"
	"encoding/json"
	"time"

	"github.com/nats-io/nats.go"
)

type NatsClient struct {
	conn *nats.Conn
}

type JobMessage struct {
	JobID    int64  `json:"job_id"`
	UploadID int64  `json:"upload_id"`
	Type     string `json:"type"`
	// optional: add more metadata later
}

func NewNatsClient(url string) (*NatsClient, error) {
	// default options: reconnects, timeout
	opts := []nats.Option{
		nats.Name("go-audio-queue-client"),
		nats.Timeout(5 * time.Second),
		nats.MaxReconnects(-1),
	}
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, err
	}
	return &NatsClient{conn: nc}, nil
}

func (n *NatsClient) Close() {
	if n.conn != nil && !n.conn.IsClosed() {
		n.conn.Close()
	}
}

func (n *NatsClient) PublishJob(ctx context.Context, subject string, jm JobMessage) error {
	b, err := json.Marshal(jm)
	if err != nil {
		return err
	}
	// use simple Publish; we don't wait for ack (NATS core). For JetStream replace with PublishMsg/JetStream.
	return n.conn.Publish(subject, b)
}

// Subscribe with a queue group; callback handles message
func (n *NatsClient) QueueSubscribe(subject, queue string, cb func(msg *nats.Msg)) (*nats.Subscription, error) {
	return n.conn.QueueSubscribe(subject, queue, cb)
}
