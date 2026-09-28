package messagequeue

import (
	"context"
	"expvar"
	"sync"
	"time"
)

type queueDepthCounter interface {
	CountQueueDepth(context.Context) (int, error)
}

var queueDepthSource struct {
	sync.RWMutex
	provider queueDepthCounter
}

func init() {
	expvar.Publish("message_queue_depth", expvar.Func(currentMessageQueueDepth))
}

func registerQueueDepthProvider(provider queueDepthCounter) {
	queueDepthSource.Lock()
	queueDepthSource.provider = provider
	queueDepthSource.Unlock()
}

func currentMessageQueueDepth() interface{} {
	queueDepthSource.RLock()
	provider := queueDepthSource.provider
	queueDepthSource.RUnlock()
	if provider == nil {
		return -1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	depth, err := provider.CountQueueDepth(ctx)
	if err != nil {
		return -1
	}
	return depth
}
