package process

import (
	"go.uber.org/zap"
	"time"
)

func (r *InteractiveRunner) createInteractiveStatusTracker(req InteractiveStartRequest, cols, rows int) *StatusTracker {
	var statusTracker *StatusTracker
	if req.StatusDetector != "" {
		detector := createStatusDetector()
		config := StatusTrackerConfig{
			Rows:            rows,
			Cols:            cols,
			CheckInterval:   req.CheckInterval,
			StabilityWindow: req.StabilityWindow,
		}
		if config.CheckInterval <= 0 {
			config.CheckInterval = 100 * time.Millisecond
		}
		// Create callback that will invoke the runner's state callback
		stateCallback := func(sessionID string, state AgentState) {
			if r.stateCallback != nil {
				r.stateCallback(sessionID, state)
			}
		}
		statusTracker = NewStatusTracker(req.SessionID, detector, stateCallback, config, r.logger)
		r.logger.Debug("status tracker created",
			zap.String("session_id", req.SessionID),
			zap.String("detector", req.StatusDetector))
	}

	return statusTracker
}
