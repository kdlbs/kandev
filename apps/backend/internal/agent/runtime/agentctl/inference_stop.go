package client

import (
	"context"
	"encoding/json"
	"io"

	"github.com/kandev/kandev/internal/agentctl/server/utility"
)

type inferenceStopObserver interface {
	ObserveInferenceStop(context.Context, *utility.AttemptSnapshot) error
}
type inferenceStopObserverKey struct{}

type detailedInferenceStopObserver interface {
	ObserveInferenceStopDetailed(context.Context, *utility.AttemptSnapshot) (*utility.InferenceStopSettlementReport, error)
}

// WithInferenceStopObserver preserves the capacity owner across native delete contexts.
func WithInferenceStopObserver(ctx context.Context, consumer InferenceConsumer) context.Context {
	if observer, ok := consumer.(inferenceStopObserver); ok {
		return context.WithValue(ctx, inferenceStopObserverKey{}, observer)
	}
	return ctx
}

func observeDeletedInference(ctx context.Context, instanceID string, body io.Reader) error {
	observer, hasObserver := ctx.Value(inferenceStopObserverKey{}).(inferenceStopObserver)
	collector, _ := ctx.Value(stopObservationKey{}).(*StopObservationCollector)
	if !hasObserver && collector == nil {
		return nil
	}
	observation := &utility.InferenceStopObservation{InstanceID: instanceID, Evidence: "unavailable"}
	if collector != nil {
		defer collector.record(observation)
	}
	var result struct {
		Attempts *utility.AttemptSnapshot `json:"inference_attempts"`
		Managed  json.RawMessage          `json:"managed_process_stop"`
	}
	if err := json.NewDecoder(io.LimitReader(body, 8<<20)).Decode(&result); err != nil || result.Attempts == nil || result.Attempts.InstanceID != instanceID {
		return nil
	}
	observation.SnapshotKnown = result.Attempts.Known
	observation.Evidence = "snapshot_only"
	var managed utility.ManagedProcessStopReceipt
	if json.Unmarshal(result.Managed, &managed) == nil && validManagedStop(&managed, result.Attempts, instanceID) {
		observation.ManagedStop = &managed
		observation.Evidence = "current_instance_scope"
	}
	if detailed, ok := observer.(detailedInferenceStopObserver); ok {
		report, err := detailed.ObserveInferenceStopDetailed(ctx, result.Attempts)
		observation.Settlement = report
		return err
	}
	if hasObserver {
		return observer.ObserveInferenceStop(ctx, result.Attempts)
	}
	return nil
}
