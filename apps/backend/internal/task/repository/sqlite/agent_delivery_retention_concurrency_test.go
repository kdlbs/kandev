package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// @covers AC-PLATFORM-DELIVERY-RETENTION-003.2
func TestDeliveryRetentionConcurrency(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, event := seedRetentionCrashEvent(t, engine)
			reached, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			repo.agentDeliveryRetentionFailpoint = func(point string) error {
				if point == "after_payload" {
					close(reached)
					<-release
				}
				return nil
			}
			cleanupDone := make(chan error, 1)
			go func() {
				_, err := repo.RunAgentDeliveryRetentionBatch(context.Background(), models.AgentDeliveryRetentionBatchOptions{})
				cleanupDone <- err
			}()
			select {
			case <-reached:
			case <-time.After(5 * time.Second):
				t.Fatal("cleanup did not reach its mutation barrier")
			}
			receiveDone := make(chan error, 1)
			go func() {
				inserted, err := repo.ReceiveAgentDeliveryEvent(context.Background(), event, event.Sequence)
				if inserted {
					err = errors.New("expired event reinserted")
				}
				receiveDone <- err
			}()
			unblock()
			if err := <-cleanupDone; err != nil {
				t.Fatal(err)
			}
			if err := <-receiveDone; !errors.Is(err, repoerrors.ErrAgentDeliveryReplayExpired) {
				t.Fatalf("receive following committed cleanup: %v", err)
			}
			cursor, err := repo.GetAgentDeliveryCursor(context.Background(), event.StreamID)
			if err != nil || cursor.RetiredSequence != event.Sequence || cursor.ProjectedSequence != event.Sequence {
				t.Fatalf("serialized checkpoint: %+v %v", cursor, err)
			}
		})
	}
}
