package statussummary

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

func cloneExecutorFailure(e *models.ExecutorFailureEpisode) *models.ExecutorFailureEpisode {
	if e == nil {
		return nil
	}
	c := *e
	c.Observation = e.Observation.Clone()
	c.ResolvedAt = cloneTimePtr(e.ResolvedAt)
	return &c
}

func (p *Projector) refreshExecutorFailure(ctx context.Context, taskID, eventType string, state *projectionState) (bool, error) {
	if p.loadExecutorFailure == nil || (state.current != nil && eventType != events.TaskUpdated) {
		return false, nil
	}
	latest, err := p.loadExecutorFailure(ctx, taskID)
	if err != nil {
		return false, err
	}
	if ExecutorFailureEqual(state.executorFailure, latest) {
		return false, nil
	}
	state.executorFailure = cloneExecutorFailure(latest)
	return true, nil
}

// ExecutorFailureEqual compares the public projection, excluding private
// ownership fences that intentionally do not survive JSON transport.
func ExecutorFailureEqual(left, right *models.ExecutorFailureEpisode) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func validateExecutorFailure(episode *models.ExecutorFailureEpisode) error {
	if episode == nil {
		return nil
	}
	if episode.Revision < 1 || episode.Observation == nil {
		return fmt.Errorf("invalid executor failure episode")
	}
	observation := episode.Observation
	fields := []struct {
		name, value string
		limit       int
	}{
		{"executor failure id", episode.ID, 256}, {"executor failure task", episode.TaskID, 256},
		{"executor failure environment", episode.EnvironmentID, 256}, {"executor failure session", episode.SessionID, 256},
		{"executor reason", observation.Reason, 128}, {"executor message", observation.Message, 768},
		{"executor runtime", observation.Runtime, 64}, {"executor pod phase", observation.PodPhase, 32},
	}
	for _, field := range fields {
		if err := validateUTF8Bytes(field.name, field.value, field.limit); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(observation)
	if err != nil {
		return err
	}
	if len(raw) > 4096 || len(observation.Containers) > 8 {
		return fmt.Errorf("executor failure evidence exceeds diagnostic budget")
	}
	return nil
}
