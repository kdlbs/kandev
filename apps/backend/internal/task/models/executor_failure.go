package models

import "time"

// ExecutorObservation contains only bounded, sanitized resource evidence.
// ResourceKey is internal ownership correlation and is never projected publicly.
type ExecutorObservation struct {
	Outcome        string                         `json:"outcome"`
	Runtime        string                         `json:"runtime"`
	ResourceKey    string                         `json:"-"`
	ObservedAt     time.Time                      `json:"observed_at"`
	OccurredAt     *time.Time                     `json:"occurred_at,omitempty"`
	Reason         string                         `json:"reason,omitempty"`
	Message        string                         `json:"message,omitempty"`
	PodPhase       string                         `json:"pod_phase,omitempty"`
	ContainerReady bool                           `json:"container_ready"`
	Restarts       int32                          `json:"restarts,omitempty"`
	Workspace      string                         `json:"workspace"`
	PodConditions  []ExecutorPodConditionEvidence `json:"pod_conditions,omitempty"`
	Containers     []ExecutorContainerEvidence    `json:"containers,omitempty"`
	Secondary      []ExecutorOperationEvidence    `json:"secondary,omitempty"`
}

type ExecutorPodConditionEvidence struct {
	Message      string     `json:"message,omitempty"`
	Type         string     `json:"type"`
	Status       string     `json:"status"`
	Reason       string     `json:"reason,omitempty"`
	TransitionAt *time.Time `json:"transition_at,omitempty"`
}

type ExecutorOperationEvidence struct {
	Operation  string    `json:"operation"`
	Reason     string    `json:"reason"`
	OccurredAt time.Time `json:"occurred_at"`
}

type ExecutorContainerEvidence struct {
	Name           string     `json:"name"`
	State          string     `json:"state"`
	Ready          bool       `json:"ready"`
	Restarts       int32      `json:"restarts,omitempty"`
	Reason         string     `json:"reason,omitempty"`
	ExitCode       *int32     `json:"exit_code,omitempty"`
	Signal         *int32     `json:"signal,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	LastExitCode   *int32     `json:"last_exit_code,omitempty"`
	LastFinishedAt *time.Time `json:"last_finished_at,omitempty"`
}

// Clone keeps cached provider evidence independent of callers and subscribers.
func (o *ExecutorObservation) Clone() *ExecutorObservation {
	if o == nil {
		return nil
	}
	c := *o
	c.OccurredAt = copyEvidenceValue(o.OccurredAt)
	c.PodConditions = append([]ExecutorPodConditionEvidence(nil), o.PodConditions...)
	for i := range c.PodConditions {
		c.PodConditions[i].TransitionAt = copyEvidenceValue(c.PodConditions[i].TransitionAt)
	}
	c.Secondary = append([]ExecutorOperationEvidence(nil), o.Secondary...)
	c.Containers = append([]ExecutorContainerEvidence(nil), o.Containers...)
	for i := range c.Containers {
		e := &c.Containers[i]
		e.ExitCode = copyEvidenceValue(e.ExitCode)
		e.Signal = copyEvidenceValue(e.Signal)
		e.LastExitCode = copyEvidenceValue(e.LastExitCode)
		e.StartedAt = copyEvidenceValue(e.StartedAt)
		e.FinishedAt = copyEvidenceValue(e.FinishedAt)
		e.LastFinishedAt = copyEvidenceValue(e.LastFinishedAt)
	}
	return &c
}

func copyEvidenceValue[T any](v *T) *T {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

// ExecutorObservationTarget is an internal immutable inspection/admission fence.
type ExecutorObservationTarget struct {
	LocalPID                  int
	AuthoritySessionID        string
	Cursor                    string
	InventoryRevision         int64
	ExpectedExecutorUpdatedAt time.Time
	TaskID                    string
	EnvironmentID             string
	SessionID                 string
	ExecutionID               string
	OwnershipGeneration       int64
	Runtime                   string
	ResourceKey               string
	ContainerID               string
	Metadata                  map[string]interface{}
}

// ExecutorFailureEpisode remains available after resource recovery and reload.
type ExecutorFailureEpisode struct {
	ID                  string               `json:"id"`
	TaskID              string               `json:"task_id"`
	EnvironmentID       string               `json:"environment_id,omitempty"`
	SessionID           string               `json:"session_id,omitempty"`
	OwnershipGeneration int64                `json:"-"`
	ResourceKey         string               `json:"-"`
	Revision            int64                `json:"revision"`
	CurrentOutcome      string               `json:"current_outcome,omitempty"`
	State               string               `json:"state"`
	FirstObservedAt     time.Time            `json:"first_observed_at"`
	LastObservedAt      time.Time            `json:"last_observed_at"`
	Observation         *ExecutorObservation `json:"observation"`
	ResolvedAt          *time.Time           `json:"resolved_at,omitempty"`
}

// ExecutorFailureAffectedSession captures settlement ownership at admission.
type ExecutorFailureAffectedSession struct {
	EnvironmentID       string
	ResourceKey         string
	OwnershipGeneration int64
	EpisodeID           string
	Candidate           ActiveSessionRecoveryCandidate
}

const (
	ExecutorOutcomeHealthy    = "healthy"
	ExecutorOutcomeUnknown    = "unknown"
	ExecutorOutcomeTerminated = "terminated"
	ExecutorOutcomeMissing    = "missing"
	ExecutorOutcomeRestarted  = "restarted"
	ProviderConversationFresh = "fresh"
)

func (o *ExecutorObservation) ConfirmedLoss() bool {
	return o != nil && (o.Outcome == ExecutorOutcomeTerminated || o.Outcome == ExecutorOutcomeMissing || o.Outcome == ExecutorOutcomeRestarted)
}

const WorkspaceRetained = "retained"

// ReportedUnavailable preserves explicit worker evidence without claiming process death.
func (o *ExecutorObservation) ReportedUnavailable() bool {
	return o != nil && o.Runtime == "k8s" && o.Outcome == ExecutorOutcomeUnknown && o.Reason == "WorkerUnavailable"
}
