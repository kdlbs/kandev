package sqlite

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// This path exists in both the incident build and the current checkout.
// Fixed message lengths separate payload cost from continually growing history.
func BenchmarkWriterMessageUpdate(b *testing.B) {
	for _, size := range []int{16384, 1048576, 16777216} {
		b.Run(fmt.Sprintf("bytes=%d", size), func(b *testing.B) { benchmarkWriterMessageUpdate(b, size) })
	}
}

func benchmarkWriterMessageUpdate(b *testing.B, size int) {
	w := newWriterWorkload(b, 8, size, 1)
	messages := make([]*models.Message, w.sessions)
	for i := range messages {
		id := fmt.Sprintf("w%d", i)
		messages[i] = &models.Message{ID: id, TaskID: id, TaskSessionID: id, TurnID: id,
			AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeMessage, Content: strings.Repeat("x", size)}
		if err := w.repo.CreateMessage(context.Background(), messages[i]); err != nil {
			b.Fatal(err)
		}
	}
	o := newWriterWorkloadObservation()
	ctx := context.WithValue(context.Background(), writerWorkloadObservationKey{}, o)
	before := w.repo.db.Stats()
	var next atomic.Int64
	var workers sync.WaitGroup
	errors := make(chan error, w.sessions)
	b.ResetTimer()
	started := time.Now()
	stopReaders := startWriterWorkloadReaders(ctx, w)
	for _, message := range messages {
		workers.Add(1)
		go func(message *models.Message) {
			defer workers.Done()
			for i := next.Add(1); i <= int64(b.N); i = next.Add(1) {
				// Vary the first byte while keeping the payload size fixed.
				message.Content = string(rune('a'+i%26)) + message.Content[1:]
				if err := writerWorkloadMeasure(ctx, "message_update", func() error { return w.repo.UpdateMessage(ctx, message) }); err != nil {
					errors <- err
					return
				}
			}
		}(message)
	}
	workers.Wait()
	elapsed := time.Since(started)
	b.StopTimer()
	stopReaders()
	close(errors)
	for err := range errors {
		b.Fatal(err)
	}
	after := w.repo.db.Stats()
	if err := w.health.Check(ctx); err != nil {
		b.Fatal(err)
	}
	o.report(b, map[string]any{"workload": "message_update", "sessions": w.sessions, "message_bytes": size,
		"updates": b.N, "elapsed_seconds": elapsed.Seconds(), "writer_wait_count": after.WaitCount - before.WaitCount,
		"writer_wait_ms": float64(after.WaitDuration-before.WaitDuration) / float64(time.Millisecond)})
}
