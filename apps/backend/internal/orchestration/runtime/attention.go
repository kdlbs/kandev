package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/redaction"
	"github.com/kandev/kandev/internal/orchestration/models"
	"slices"
	"time"
)

type AttentionReader interface {
	ReadAttention(context.Context, *models.AssistantBinding, string, []models.Attention) ([]models.AttentionSource, error)
}

const attentionBatch = 100

func (s *Service) attentionNow() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Called by the existing run scheduler, with bounded continuation between ticks.
func (s *Service) ReconcileAttention(ctx context.Context) error {
	s.attentionMu.Lock()
	defer s.attentionMu.Unlock()
	if s.Attention == nil {
		return nil
	}
	now := s.attentionNow()
	if now.Before(s.attentionNext) {
		return nil
	}
	if s.attentionAfter == "" {
		if err := s.Repo.PruneFriction(ctx, now); err != nil {
			return err
		}
	}
	targets, err := s.Repo.AttentionTargets(ctx, "", s.attentionAfter, attentionBatch)
	if err != nil {
		return err
	}
	var failures []error
	for _, target := range targets {
		if err := s.reconcileAttentionTarget(ctx, target); err != nil {
			failures = append(failures, err)
		}
	}
	if len(targets) == attentionBatch {
		s.attentionAfter = targets[len(targets)-1].Cursor
	} else {
		s.attentionAfter = ""
		s.attentionNext = now.Add(time.Minute)
	}
	return errors.Join(failures...)
}

func (s *Service) ReconcileAttentionTask(ctx context.Context, task string) error {
	s.attentionMu.Lock()
	defer s.attentionMu.Unlock()
	if s.Attention == nil || task == "" {
		return nil
	}
	targets, err := s.Repo.AttentionTargets(ctx, task, "", attentionBatch)
	if err != nil {
		return err
	}
	var failures []error
	for _, target := range targets {
		if err := s.reconcileAttentionTarget(ctx, target); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (s *Service) reconcileAttentionTarget(ctx context.Context, target models.AttentionTarget) error {
	b, err := s.Repo.AssistantBindingByID(ctx, target.BindingID)
	if err != nil {
		return err
	}
	ctx = authn.WithIdentity(ctx, authn.Identity{UserID: b.OwnerUserID, Role: authn.RoleMember})
	previous, err := s.Repo.AttentionForTask(ctx, b.ID, target.TaskID)
	if err != nil {
		return err
	}
	sourceCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	sources, readErr := s.Attention.ReadAttention(sourceCtx, b, target.TaskID, previous)
	cancel()
	if readErr != nil {
		sources = unknownAttentionSources(previous)
	} else {
		sources = completeAttentionSources(sources, previous)
	}
	if err = validateAttentionSources(sources); err != nil {
		return err
	}
	changed, err := s.Repo.ProjectAttention(ctx, b, target.TaskID, sources, s.attentionNow())
	if err != nil {
		return err
	}
	var frictionErr error
	if readErr == nil {
		frictionErr = s.observeFriction(ctx, b, target.TaskID, sources)
	}
	if changed && s.AttentionUpdated != nil {
		s.AttentionUpdated(ctx, b.ID, s.attentionNow())
	}
	if readErr != nil {
		return fmt.Errorf("attention source unavailable")
	}
	if !s.AssistantEnabled {
		return nil
	}
	a, err := s.Personas.GetAgentInstance(ctx, b.OrchestratorID)
	if err != nil {
		return err
	}
	if paused(a) {
		return frictionErr
	}
	return errors.Join(s.dispatchAttentionWakes(ctx, b, target.TaskID), frictionErr)
}
func unknownAttentionSources(previous []models.Attention) []models.AttentionSource {
	sources := make([]models.AttentionSource, 0, len(previous)+1)
	for _, row := range previous {
		source := row.AttentionSource
		if source.SourceID == "source-health" {
			continue
		}
		if source.State == models.AttentionPending {
			source.State = statusUnknown
		}
		sources = append(sources, source)
	}
	return append(sources, models.AttentionSource{SourceID: "source-health", Kind: attentionKindFailure, State: statusUnknown, SourceRevision: healthUnavailable, Summary: "Current task attention is unavailable. Open the native task to check its state."})
}
func completeAttentionSources(sources []models.AttentionSource, previous []models.Attention) []models.AttentionSource {
	seen := map[string]bool{}
	for _, source := range sources {
		seen[source.SessionID+":"+source.Kind+":"+source.SourceID] = true
	}
	for _, row := range previous {
		source := row.AttentionSource
		if seen[source.SessionID+":"+source.Kind+":"+source.SourceID] {
			continue
		}
		source.State = "inactive"
		if source.Kind == attentionKindQuestion || source.Kind == attentionKindPermission {
			source.State = models.AttentionExpired
		}
		sources = append(sources, source)
	}
	return sources
}

func validateAttentionSources(sources []models.AttentionSource) error {
	if len(sources) > 1000 {
		return fmt.Errorf("attention source budget exceeded")
	}
	seen := map[string]bool{}
	for i := range sources {
		s := &sources[i]
		key := s.SessionID + ":" + s.Kind + ":" + s.SourceID
		if seen[key] || s.SourceID == "" || len(s.SourceID) > 1024 || len(s.SessionID) > 200 || s.SourceRevision == "" || len(s.SourceRevision) > 256 {
			return fmt.Errorf("invalid attention identity")
		}
		if !slices.Contains([]string{attentionKindQuestion, attentionKindPermission, attentionKindAuthentication, attentionKindFailure, "review", "result"}, s.Kind) || !slices.Contains([]string{models.AttentionPending, statusResolved, "expired", statusUnknown, "inactive"}, s.State) {
			return fmt.Errorf("invalid attention state")
		}
		seen[key] = true
		s.Summary = clip(redaction.NewRedactor().String(s.Summary), 500)
	}
	return nil
}

type attentionWakeRef struct {
	ID             string `json:"id"`
	SourceRevision string `json:"source_revision"`
}

func (s *Service) dispatchAttentionWakes(ctx context.Context, b *models.AssistantBinding, task string) error {
	wakes, err := s.Repo.PendingAttentionWakes(ctx, b.ID, task)
	if err != nil {
		return err
	}
	for _, wake := range wakes {
		refs := []attentionWakeRef{{ID: wake.AttentionID, SourceRevision: wake.SourceRevision}}
		payload := map[string]any{"attention_refs": refs, "callback": map[string]string{taskIDKey: task, "reason": "attention_changed"}}
		if err = s.QueueTurn(ctx, b.OrchestratorID, b.ConversationID, "assistant_attention", "assistant-attention:"+wake.ID, payload); err != nil {
			return err
		}
		if err = s.Repo.AcknowledgeAttentionWake(ctx, wake); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) SetAttentionReader(reader AttentionReader) {
	s.attentionMu.Lock()
	defer s.attentionMu.Unlock()
	s.Attention = reader
}
func (s *Service) validateAttentionWake(ctx context.Context, payload map[string]any) error {
	value, exists := payload["attention_refs"]
	if !exists {
		return nil
	}
	raw, _ := json.Marshal(value)
	var refs []attentionWakeRef
	if json.Unmarshal(raw, &refs) != nil || len(refs) == 0 || len(refs) > attentionBatch {
		return fmt.Errorf("invalid attention wake")
	}
	binding, _ := payload["binding_id"].(string)
	b, err := s.Repo.AssistantBindingByID(ctx, binding)
	if err != nil {
		return err
	}
	ctx = authn.WithIdentity(ctx, authn.Identity{UserID: b.OwnerUserID, Role: authn.RoleMember})
	current := false
	refreshed := map[string]bool{}
	for _, ref := range refs {
		row, err := s.Repo.AttentionByID(ctx, binding, ref.ID)
		if err != nil {
			return err
		}
		if !s.attentionVisible(ctx, b, *row) {
			return fmt.Errorf("attention task no longer managed")
		}
		if !refreshed[row.TaskID] {
			if err = s.ReconcileAttentionTask(ctx, row.TaskID); err != nil {
				return err
			}
			refreshed[row.TaskID] = true
		}
		row, err = s.Repo.AttentionByID(ctx, binding, ref.ID)
		if err == nil && row.State == models.AttentionPending && row.SourceRevision == ref.SourceRevision {
			current = true
		}
	}
	if !current {
		return fmt.Errorf("attention request no longer current")
	}
	return nil
}
