package bus

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// MemoryEventBus implements EventBus using in-memory channels
type MemoryEventBus struct {
	subscriptions map[string][]*memorySubscription
	queues        map[string]*queueGroup // For queue subscriptions
	mu            sync.RWMutex
	logger        *logger.Logger
	closed        bool
}

// memorySubscription represents an in-memory subscription
type memorySubscription struct {
	bus      *MemoryEventBus
	subject  string
	pattern  *regexp.Regexp // For wildcard matching
	handler  EventHandler
	queue    string // Empty for regular subscriptions
	active   bool
	mu       sync.Mutex
}

// queueGroup manages load balancing for queue subscriptions
type queueGroup struct {
	subscribers []*memorySubscription
	nextIndex   int
	mu          sync.Mutex
}

// Unsubscribe removes the subscription
func (s *memorySubscription) Unsubscribe() error {
	s.mu.Lock()
	s.active = false
	s.mu.Unlock()

	// Remove from bus subscriptions
	s.bus.mu.Lock()
	defer s.bus.mu.Unlock()

	if subs, ok := s.bus.subscriptions[s.subject]; ok {
		for i, sub := range subs {
			if sub == s {
				s.bus.subscriptions[s.subject] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
	}

	// Remove from queue group if applicable
	if s.queue != "" {
		queueKey := s.queue + ":" + s.subject
		if qg, ok := s.bus.queues[queueKey]; ok {
			qg.mu.Lock()
			for i, sub := range qg.subscribers {
				if sub == s {
					qg.subscribers = append(qg.subscribers[:i], qg.subscribers[i+1:]...)
					break
				}
			}
			qg.mu.Unlock()
		}
	}

	return nil
}

// IsValid returns whether the subscription is still active
func (s *memorySubscription) IsValid() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

// NewMemoryEventBus creates a new in-memory event bus
func NewMemoryEventBus(log *logger.Logger) *MemoryEventBus {
	return &MemoryEventBus{
		subscriptions: make(map[string][]*memorySubscription),
		queues:        make(map[string]*queueGroup),
		logger:        log,
	}
}

// Publish sends an event to all matching subscribers
func (b *MemoryEventBus) Publish(ctx context.Context, subject string, event *Event) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return fmt.Errorf("event bus is closed")
	}

	// Track which queue groups we've already delivered to
	deliveredQueues := make(map[string]bool)

	// Find all matching subscriptions
	for pattern, subs := range b.subscriptions {
		for _, sub := range subs {
			sub.mu.Lock()
			active := sub.active
			sub.mu.Unlock()

			if !active {
				continue
			}

			if !b.matches(subject, pattern, sub.pattern) {
				continue
			}

			// If it's a queue subscription, use the queue group (only once per group)
			if sub.queue != "" {
				queueKey := sub.queue + ":" + pattern
				if !deliveredQueues[queueKey] {
					deliveredQueues[queueKey] = true
					b.publishToQueue(ctx, queueKey, subject, event)
				}
				continue
			}

			// Regular subscription - deliver to all
			go func(s *memorySubscription, e *Event) {
				if err := s.handler(ctx, e); err != nil {
					b.logger.Error("Event handler error",
						zap.String("subject", subject),
						zap.Error(err))
				}
			}(sub, event)
		}
	}

	b.logger.Debug("Published event",
		zap.String("subject", subject),
		zap.String("event_id", event.ID),
		zap.String("event_type", event.Type))

	return nil
}

// Subscribe creates a subscription to a subject pattern
func (b *MemoryEventBus) Subscribe(subject string, handler EventHandler) (Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil, fmt.Errorf("event bus is closed")
	}

	sub := &memorySubscription{
		bus:     b,
		subject: subject,
		pattern: compilePattern(subject),
		handler: handler,
		active:  true,
	}

	b.subscriptions[subject] = append(b.subscriptions[subject], sub)

	b.logger.Info("Subscribed to subject", zap.String("subject", subject))
	return sub, nil
}

// QueueSubscribe creates a queue subscription for load balancing
// Only one subscriber in the queue group receives each message
func (b *MemoryEventBus) QueueSubscribe(subject, queue string, handler EventHandler) (Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil, fmt.Errorf("event bus is closed")
	}

	sub := &memorySubscription{
		bus:     b,
		subject: subject,
		pattern: compilePattern(subject),
		handler: handler,
		queue:   queue,
		active:  true,
	}

	b.subscriptions[subject] = append(b.subscriptions[subject], sub)

	// Add to queue group
	queueKey := queue + ":" + subject
	if _, ok := b.queues[queueKey]; !ok {
		b.queues[queueKey] = &queueGroup{
			subscribers: make([]*memorySubscription, 0),
		}
	}
	b.queues[queueKey].subscribers = append(b.queues[queueKey].subscribers, sub)

	b.logger.Info("Queue subscribed to subject",
		zap.String("subject", subject),
		zap.String("queue", queue))
	return sub, nil
}

// Request sends a request and waits for a response
func (b *MemoryEventBus) Request(ctx context.Context, subject string, event *Event, timeout time.Duration) (*Event, error) {
	// For in-memory bus, we implement a simple request-reply pattern
	// Create a unique reply subject
	replySubject := fmt.Sprintf("_INBOX.%s", event.ID)

	// Channel to receive the response
	responseChan := make(chan *Event, 1)
	errChan := make(chan error, 1)

	// Subscribe to the reply subject
	sub, err := b.Subscribe(replySubject, func(ctx context.Context, e *Event) error {
		responseChan <- e
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create reply subscription: %w", err)
	}
	defer func() {
		_ = sub.Unsubscribe()
	}()

	// Add reply subject to event data
	// We need to handle the case where Data is a struct or a map
	switch data := event.Data.(type) {
	case map[string]interface{}:
		if data == nil {
			data = make(map[string]interface{})
		}
		data["_reply"] = replySubject
		event.Data = data
	case nil:
		event.Data = map[string]interface{}{"_reply": replySubject}
	default:
		// For struct types, wrap in a map with the original data and reply subject
		event.Data = map[string]interface{}{
			"data":   data,
			"_reply": replySubject,
		}
	}

	// Publish the request
	if err := b.Publish(ctx, subject, event); err != nil {
		return nil, fmt.Errorf("failed to publish request: %w", err)
	}

	// Wait for response with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case response := <-responseChan:
		return response, nil
	case err := <-errChan:
		return nil, err
	case <-timeoutCtx.Done():
		return nil, fmt.Errorf("request timeout after %v", timeout)
	}
}

// Close closes the event bus
func (b *MemoryEventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closed = true

	// Deactivate all subscriptions
	for _, subs := range b.subscriptions {
		for _, sub := range subs {
			sub.mu.Lock()
			sub.active = false
			sub.mu.Unlock()
		}
	}

	b.subscriptions = make(map[string][]*memorySubscription)
	b.queues = make(map[string]*queueGroup)

	b.logger.Info("Memory event bus closed")
}

// IsConnected returns true (always connected for in-memory)
func (b *MemoryEventBus) IsConnected() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return !b.closed
}

// matches checks if a subject matches a pattern
// Supports NATS-style wildcards: * (single token) and > (multiple tokens)
func (b *MemoryEventBus) matches(subject, pattern string, regex *regexp.Regexp) bool {
	// If no wildcards, do exact match
	if !strings.Contains(pattern, "*") && !strings.Contains(pattern, ">") {
		return subject == pattern
	}

	// Use the compiled regex
	if regex != nil {
		return regex.MatchString(subject)
	}

	return false
}

// compilePattern converts NATS-style pattern to regex
func compilePattern(pattern string) *regexp.Regexp {
	// If no wildcards, no need for regex
	if !strings.Contains(pattern, "*") && !strings.Contains(pattern, ">") {
		return nil
	}

	// Escape special regex characters except * and >
	escaped := regexp.QuoteMeta(pattern)

	// Replace escaped \* with regex for single token (anything except .)
	escaped = strings.ReplaceAll(escaped, `\*`, `[^.]+`)

	// Replace escaped \> with regex for remaining tokens (anything)
	escaped = strings.ReplaceAll(escaped, `\>`, `.+`)

	// Anchor the pattern
	escaped = "^" + escaped + "$"

	regex, err := regexp.Compile(escaped)
	if err != nil {
		return nil
	}

	return regex
}

// publishToQueue delivers to one subscriber in the queue group (round-robin)
func (b *MemoryEventBus) publishToQueue(ctx context.Context, queueKey, subject string, event *Event) {
	qg, ok := b.queues[queueKey]
	if !ok {
		return
	}

	qg.mu.Lock()
	defer qg.mu.Unlock()

	if len(qg.subscribers) == 0 {
		return
	}

	// Find next active subscriber (round-robin)
	startIndex := qg.nextIndex
	for i := 0; i < len(qg.subscribers); i++ {
		idx := (startIndex + i) % len(qg.subscribers)
		sub := qg.subscribers[idx]

		sub.mu.Lock()
		active := sub.active
		sub.mu.Unlock()

		if active {
			qg.nextIndex = (idx + 1) % len(qg.subscribers)

			go func(s *memorySubscription, e *Event) {
				if err := s.handler(ctx, e); err != nil {
					b.logger.Error("Queue event handler error",
						zap.String("subject", subject),
						zap.String("queue", queueKey),
						zap.Error(err))
				}
			}(sub, event)
			return
		}
	}
}
