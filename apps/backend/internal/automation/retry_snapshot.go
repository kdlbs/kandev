package automation

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const RetryLaunchConfigVersion int64 = 1

type RetryLaunchConfigSnapshot struct {
	Version            int64                  `json:"version"`
	AutomationID       string                 `json:"automation_id"`
	WorkspaceID        string                 `json:"workspace_id"`
	Name               string                 `json:"name"`
	WorkflowID         string                 `json:"workflow_id"`
	WorkflowStepID     string                 `json:"workflow_step_id"`
	AgentProfileID     string                 `json:"agent_profile_id"`
	ExecutorProfileID  string                 `json:"executor_profile_id"`
	Prompt             string                 `json:"prompt"`
	TaskTitleTemplate  string                 `json:"task_title_template"`
	TaskMode           TaskMode               `json:"task_mode"`
	RepositoryMode     RepositoryMode         `json:"repository_mode"`
	Repositories       []AutomationRepository `json:"repositories"`
	ContinuationPolicy ContinuationPolicy     `json:"continuation_policy"`
	MaxConcurrentRuns  int                    `json:"max_concurrent_runs"`
	ContinuationTaskID string                 `json:"continuation_task_id"`
	TriggerID          string                 `json:"trigger_id"`
	TriggerType        TriggerType            `json:"trigger_type"`
	TriggerData        json.RawMessage        `json:"trigger_data"`
	DedupKey           string                 `json:"dedup_key"`
	ResolvedPrompt     string                 `json:"resolved_prompt"`
	ResolvedTitle      string                 `json:"resolved_title"`
	ResolvedTriggerAt  time.Time              `json:"resolved_trigger_at"`
	RetryPolicy        RetryPolicy            `json:"retry_policy"`
}

func buildRetryLaunchConfigSnapshot(a *Automation, triggerID string, triggerType TriggerType, triggerData json.RawMessage, dedupKey string, resolvedAt time.Time) (RetryLaunchConfigSnapshot, error) {
	if a == nil || a.ID == "" || a.WorkspaceID == "" || triggerID == "" || triggerType == "" {
		return RetryLaunchConfigSnapshot{}, errors.New("retry launch snapshot identity is incomplete")
	}
	policy, err := NormalizeRetryPolicy(a.RetryPolicy)
	if err != nil {
		return RetryLaunchConfigSnapshot{}, fmt.Errorf("normalize retry policy: %w", err)
	}
	if resolvedAt.IsZero() {
		resolvedAt = time.Now().UTC()
	}
	safeTriggerData := SafeRetryTriggerProjection(triggerType, triggerID, triggerData, dedupKey)
	resolvedTriggerData := triggerData
	if triggerType == TriggerTypeWebhook {
		resolvedTriggerData = safeTriggerData
	}
	snapshot := RetryLaunchConfigSnapshot{
		Version:            RetryLaunchConfigVersion,
		AutomationID:       a.ID,
		WorkspaceID:        a.WorkspaceID,
		Name:               a.Name,
		WorkflowID:         a.WorkflowID,
		WorkflowStepID:     a.WorkflowStepID,
		AgentProfileID:     a.AgentProfileID,
		ExecutorProfileID:  a.ExecutorProfileID,
		Prompt:             a.Prompt,
		TaskTitleTemplate:  a.TaskTitleTemplate,
		TaskMode:           a.TaskMode,
		RepositoryMode:     a.RepositoryMode,
		Repositories:       append([]AutomationRepository(nil), a.Repositories...),
		ContinuationPolicy: a.ContinuationPolicy,
		MaxConcurrentRuns:  a.MaxConcurrentRuns,
		ContinuationTaskID: a.ContinuationTaskID,
		TriggerID:          triggerID,
		TriggerType:        triggerType,
		TriggerData:        safeTriggerData,
		DedupKey:           dedupKey,
		ResolvedPrompt:     InterpolateAgentPromptAt(a.Prompt, triggerType, resolvedTriggerData, resolvedAt),
		ResolvedTitle:      RenderRunDisplayTitleAt(a, triggerType, resolvedTriggerData, resolvedAt),
		ResolvedTriggerAt:  resolvedAt.UTC(),
		RetryPolicy:        policy,
	}
	return snapshot, nil
}

func encodeRetryLaunchConfigSnapshot(snapshot RetryLaunchConfigSnapshot) (string, error) {
	if snapshot.Version != RetryLaunchConfigVersion {
		return "", fmt.Errorf("unsupported retry launch snapshot version %d", snapshot.Version)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return "", fmt.Errorf("marshal retry launch snapshot: %w", err)
	}
	return string(encoded), nil
}
func DecodeRetryLaunchConfigSnapshot(raw string, version int64) (RetryLaunchConfigSnapshot, error) {
	if version != RetryLaunchConfigVersion || raw == "" || raw == "{}" {
		return RetryLaunchConfigSnapshot{}, fmt.Errorf("unsupported retry launch snapshot version %d", version)
	}
	var snapshot RetryLaunchConfigSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return RetryLaunchConfigSnapshot{}, fmt.Errorf("decode retry launch snapshot: %w", err)
	}
	if snapshot.Version != RetryLaunchConfigVersion || snapshot.AutomationID == "" || snapshot.WorkspaceID == "" || snapshot.TriggerID == "" || snapshot.TriggerType == "" || snapshot.ResolvedTriggerAt.IsZero() {
		return RetryLaunchConfigSnapshot{}, errors.New("retry launch snapshot is incomplete")
	}
	if len(snapshot.TriggerData) == 0 {
		return RetryLaunchConfigSnapshot{}, errors.New("retry launch snapshot content is incomplete")
	}
	if snapshot.MaxConcurrentRuns < 0 {
		return RetryLaunchConfigSnapshot{}, errors.New("retry launch snapshot concurrency limit is invalid")
	}
	return snapshot, nil
}
