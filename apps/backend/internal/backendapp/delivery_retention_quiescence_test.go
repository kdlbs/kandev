package backendapp

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestDeliveryRetentionQuiescence(t *testing.T) {
	var calls []string
	stopErr := errors.New("delivery workers failed to stop")
	quiesce := quiesceDeliveryRetention(
		func() { calls = append(calls, "retention") },
		func() error {
			calls = append(calls, "delivery")
			return stopErr
		},
	)
	if err := quiesce(); !errors.Is(err, stopErr) {
		t.Fatalf("quiesce error = %v, want %v", err, stopErr)
	}
	if want := []string{"retention", "delivery"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("quiescence order = %v, want %v", calls, want)
	}
}

func TestDeliveryRetentionResetQuiescence(t *testing.T) {
	worker := &resetRetentionWorker{}
	appCtx := context.Background()
	resume := pauseDeliveryRetentionForReset(appCtx, worker, nil)
	if worker.stops != 1 || worker.starts != 0 {
		t.Fatalf("before resume stops/starts = %d/%d", worker.stops, worker.starts)
	}
	resume()
	if worker.stops != 1 || worker.starts != 1 {
		t.Fatalf("after resume stops/starts = %d/%d", worker.stops, worker.starts)
	}
}

type resetRetentionWorker struct {
	starts int
	stops  int
}

func (w *resetRetentionWorker) Start(context.Context) error {
	w.starts++
	return nil
}

func (w *resetRetentionWorker) Stop() { w.stops++ }
