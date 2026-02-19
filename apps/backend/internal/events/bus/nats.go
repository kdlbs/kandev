package bus

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
)

// NATSEventBus implements EventBus using NATS
type NATSEventBus struct {
	conn   *nats.Conn
	logger *logger.Logger
	config config.NATSConfig
}

// NewNATSEventBus creates a new NATS event bus with reconnection logic
func NewNATSEventBus(cfg config.NATSConfig, log *logger.Logger) (*NATSEventBus, error) {
	bus := &NATSEventBus{
		logger: log,
		config: cfg,
	}

	opts := []nats.Option{
		nats.Name(cfg.ClientID),
		nats.MaxReconnects(cfg.MaxReconnects),
		nats.ReconnectWait(2 * time.Second),
		nats.ReconnectBufSize(5 * 1024 * 1024), // 5MB buffer during reconnect

		// Connection status handlers
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			if err != nil {
				log.Warn("NATS disconnected", zap.Error(err))
			} else {
				log.Info("NATS disconnected")
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Info("NATS reconnected", zap.String("url", nc.ConnectedUrl()))
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			if err := nc.LastError(); err != nil {
				log.Error("NATS connection closed", zap.Error(err))
			} else {
				log.Info("NATS connection closed")
			}
		}),
		nats.ErrorHandler(func(nc *nats.Conn, sub *nats.Subscription, err error) {
			log.Error("NATS error",
				zap.Error(err),
				zap.String("subject", sub.Subject),
			)
		}),
	}

	conn, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	bus.conn = conn
	log.Info("Connected to NATS", zap.String("url", cfg.URL))

	return bus, nil
}

// Publish sends an event to a subject
func (b *NATSEventBus) Publish(ctx context.Context, subject string, event *Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	if err := b.conn.Publish(subject, data); err != nil {
		b.logger.Error("Failed to publish event",
			zap.String("subject", subject),
			zap.String("event_type", event.Type),
			zap.Error(err),
		)
		return fmt.Errorf("failed to publish event: %w", err)
	}

	b.logger.Debug("Published event",
		zap.String("subject", subject),
		zap.String("event_id", event.ID),
		zap.String("event_type", event.Type),
	)

	return nil
}

// Subscribe creates a subscription to a subject pattern
func (b *NATSEventBus) Subscribe(subject string, handler EventHandler) (Subscription, error) {
	sub, err := b.conn.Subscribe(subject, b.createMsgHandler(handler))
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}

	b.logger.Debug("Subscribed to subject", zap.String("subject", subject))
	return &natsSubscription{sub: sub}, nil
}

// QueueSubscribe creates a queue subscription for load balancing
func (b *NATSEventBus) QueueSubscribe(subject, queue string, handler EventHandler) (Subscription, error) {
	sub, err := b.conn.QueueSubscribe(subject, queue, b.createMsgHandler(handler))
	if err != nil {
		return nil, fmt.Errorf("failed to queue subscribe to %s: %w", subject, err)
	}

	b.logger.Debug("Queue subscribed to subject",
		zap.String("subject", subject),
		zap.String("queue", queue),
	)
	return &natsSubscription{sub: sub}, nil
}

// createMsgHandler creates a NATS message handler from an EventHandler
func (b *NATSEventBus) createMsgHandler(handler EventHandler) nats.MsgHandler {
	return func(msg *nats.Msg) {
		var event Event
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			b.logger.Error("Failed to unmarshal event",
				zap.String("subject", msg.Subject),
				zap.Error(err),
			)
			return
		}

		ctx := context.Background()
		if err := handler(ctx, &event); err != nil {
			b.logger.Error("Event handler failed",
				zap.String("subject", msg.Subject),
				zap.String("event_id", event.ID),
				zap.String("event_type", event.Type),
				zap.Error(err),
			)
		}
	}
}

// Request sends a request and waits for a response (with timeout)
func (b *NATSEventBus) Request(ctx context.Context, subject string, event *Event, timeout time.Duration) (*Event, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request event: %w", err)
	}

	msg, err := b.conn.Request(subject, data, timeout)
	if err != nil {
		b.logger.Error("Request failed",
			zap.String("subject", subject),
			zap.String("event_type", event.Type),
			zap.Error(err),
		)
		return nil, fmt.Errorf("request to %s failed: %w", subject, err)
	}

	var response Event
	if err := json.Unmarshal(msg.Data, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &response, nil
}

// Close closes the NATS connection gracefully
func (b *NATSEventBus) Close() {
	if b.conn != nil {
		// Drain will process pending messages before closing
		if err := b.conn.Drain(); err != nil {
			b.logger.Warn("Error draining NATS connection", zap.Error(err))
			// Fall back to regular close
			b.conn.Close()
		}
		b.logger.Info("NATS connection closed")
	}
}

// IsConnected returns whether the NATS connection is active
func (b *NATSEventBus) IsConnected() bool {
	if b.conn == nil {
		return false
	}
	return b.conn.IsConnected()
}
