package webpush

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	deliveryQueueCapacity = 16
	deliveryQueueWorkers  = 4
	deliveryJobTimeout    = 30 * time.Second
)

type deliveryJob struct {
	userID  string
	payload Payload
	opts    SendOptions
}

type deliveryQueue struct {
	mu        sync.RWMutex
	closed    bool
	closeOnce sync.Once
	jobs      chan deliveryJob
	workers   sync.WaitGroup
	service   *Service
}

func newDeliveryQueue(service *Service) *deliveryQueue {
	queue := &deliveryQueue{
		jobs:    make(chan deliveryJob, deliveryQueueCapacity),
		service: service,
	}
	queue.workers.Add(deliveryQueueWorkers)
	for worker := 0; worker < deliveryQueueWorkers; worker++ {
		go queue.run()
	}
	return queue
}

func (q *deliveryQueue) Enqueue(ctx context.Context, userID string, payload Payload, opts SendOptions) error {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.closed {
		return ErrDeliveryQueueUnavailable
	}
	job := deliveryJob{userID: userID, payload: payload, opts: opts}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case q.jobs <- job:
		return nil
	default:
		return ErrDeliveryQueueFull
	}
}

func (q *deliveryQueue) Close() error {
	q.closeOnce.Do(func() {
		q.mu.Lock()
		q.closed = true
		close(q.jobs)
		q.mu.Unlock()
		q.workers.Wait()
	})
	return nil
}

func (q *deliveryQueue) run() {
	defer q.workers.Done()
	for job := range q.jobs {
		ctx, cancel := context.WithTimeout(context.Background(), deliveryJobTimeout)
		delivered, err := q.service.Notify(ctx, job.userID, job.payload, job.opts)
		cancel()
		if err != nil {
			q.service.logger.Warn("queued push delivery failed",
				zap.String("user_id", job.userID),
				zap.Int("delivered", delivered),
				zap.Error(err))
		}
	}
}
