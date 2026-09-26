package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
)

// ExactRetirementPredicate is a closed inventory category. New evidence must
// be added explicitly so an unavailable owner cannot appear safe by omission.
type ExactRetirementPredicate string

const (
	ExactRetirementIdentityPredicate     ExactRetirementPredicate = "identity"
	ExactRetirementQueuePredicate        ExactRetirementPredicate = "session_queue"
	ExactRetirementMovePredicate         ExactRetirementPredicate = "move_dispatch"
	ExactRetirementRelationshipPredicate ExactRetirementPredicate = "relationships"
	ExactRetirementPRPredicate           ExactRetirementPredicate = "pr_watch"
	ExactRetirementPreservationPredicate ExactRetirementPredicate = "preservation"
	ExactRetirementGitPredicate          ExactRetirementPredicate = "git"
	ExactRetirementEnvironmentPredicate  ExactRetirementPredicate = "environment_runtime"
	ExactRetirementConsumerPredicate     ExactRetirementPredicate = "lease_consumer"
	ExactRetirementOwnershipPredicate    ExactRetirementPredicate = "replacement_ownership"
)

type ExactRetirementReceiptStatus string

const (
	ExactRetirementReceiptPass    ExactRetirementReceiptStatus = "PASS"
	ExactRetirementReceiptBlocked ExactRetirementReceiptStatus = "BLOCKED"
	ExactRetirementReceiptUnknown ExactRetirementReceiptStatus = "UNKNOWN"
)

var (
	ErrExactRetirementPairInvalid      = errors.New("exact retirement pair is invalid")
	ErrExactRetirementGenerationStale  = errors.New("exact retirement generation is stale")
	ErrExactRetirementWorkspaceInvalid = errors.New("exact retirement workspace is invalid")
)

type ExactRetirementPreviewRequest struct {
	OldTaskID                     string
	ReplacementTaskID             string
	WorkspaceID                   string
	ExpectedOldGeneration         string
	ExpectedReplacementGeneration string
}

type ExactRetirementPredicateReceipt struct {
	Predicate          ExactRetirementPredicate     `json:"predicate"`
	Status             ExactRetirementReceiptStatus `json:"status"`
	ReasonCode         string                       `json:"reason_code"`
	ResourceID         string                       `json:"resource_id"`
	ObservedGeneration string                       `json:"observed_generation"`
	EvidenceDigest     string                       `json:"evidence_digest"`
}

type ExactRetirementPreview struct {
	OldTaskID         string                            `json:"old_task_id"`
	ReplacementTaskID string                            `json:"replacement_task_id"`
	WorkspaceID       string                            `json:"workspace_id"`
	Eligible          bool                              `json:"eligible"`
	Receipts          []ExactRetirementPredicateReceipt `json:"receipts"`
}

func exactRetirementGeneration(task *models.Task) string {
	return task.UpdatedAt.UTC().Format(time.RFC3339Nano)
}

func exactRetirementDigest(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func exactRetirementReceiptsEligible(receipts []ExactRetirementPredicateReceipt) bool {
	if len(receipts) == 0 {
		return false
	}
	for _, receipt := range receipts {
		if receipt.Status != ExactRetirementReceiptPass {
			return false
		}
	}
	return true
}

func (s *Service) authorizeExactRetirementPreview(ctx context.Context, replacementTaskID string) error {
	if err := s.AuthorizeTaskScope(ctx, replacementTaskID, authz.ScopeTaskWrite); err != nil {
		return err
	}
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || !identity.IsAdmin() {
		return ErrForbidden
	}
	return nil
}

// PreviewExactRetirement only reads authorized task rows. Evidence owners are
// intentionally represented as UNKNOWN until their read-only adapters exist.
func (s *Service) PreviewExactRetirement(ctx context.Context, request ExactRetirementPreviewRequest) (*ExactRetirementPreview, error) {
	if request.OldTaskID == "" || request.ReplacementTaskID == "" {
		return nil, ErrExactRetirementPairInvalid
	}
	oldTask, err := s.GetTask(ctx, request.OldTaskID)
	if err != nil {
		return nil, err
	}
	if err := s.AuthorizeTaskScope(ctx, oldTask.ID, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	replacementTask, err := s.GetTask(ctx, request.ReplacementTaskID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeExactRetirementPreview(ctx, replacementTask.ID); err != nil {
		return nil, err
	}
	if oldTask.ID == replacementTask.ID {
		return nil, ErrExactRetirementPairInvalid
	}
	if request.WorkspaceID == "" || oldTask.WorkspaceID != request.WorkspaceID || replacementTask.WorkspaceID != request.WorkspaceID {
		return nil, ErrExactRetirementWorkspaceInvalid
	}
	oldGeneration := exactRetirementGeneration(oldTask)
	replacementGeneration := exactRetirementGeneration(replacementTask)
	if request.ExpectedOldGeneration != oldGeneration || request.ExpectedReplacementGeneration != replacementGeneration {
		return nil, ErrExactRetirementGenerationStale
	}
	receipts := []ExactRetirementPredicateReceipt{{
		Predicate: ExactRetirementIdentityPredicate, Status: ExactRetirementReceiptPass,
		ReasonCode: "EXACT_PAIR_AUTHORIZED", ResourceID: oldTask.ID,
		ObservedGeneration: oldGeneration,
		EvidenceDigest:     exactRetirementDigest(oldTask.ID, replacementTask.ID, request.WorkspaceID, oldGeneration, replacementGeneration),
	}}
	for _, predicate := range []ExactRetirementPredicate{
		ExactRetirementQueuePredicate, ExactRetirementMovePredicate, ExactRetirementRelationshipPredicate,
		ExactRetirementPRPredicate, ExactRetirementPreservationPredicate, ExactRetirementGitPredicate,
		ExactRetirementEnvironmentPredicate, ExactRetirementConsumerPredicate, ExactRetirementOwnershipPredicate,
	} {
		receipts = append(receipts, ExactRetirementPredicateReceipt{
			Predicate: predicate, Status: ExactRetirementReceiptUnknown, ReasonCode: "INVENTORY_UNAVAILABLE",
			ResourceID: oldTask.ID, ObservedGeneration: oldGeneration,
			EvidenceDigest: exactRetirementDigest(string(predicate), oldTask.ID, replacementTask.ID, oldGeneration),
		})
	}
	sort.Slice(receipts, func(i, j int) bool { return receipts[i].Predicate < receipts[j].Predicate })
	return &ExactRetirementPreview{
		OldTaskID: oldTask.ID, ReplacementTaskID: replacementTask.ID, WorkspaceID: request.WorkspaceID,
		Eligible: exactRetirementReceiptsEligible(receipts), Receipts: receipts,
	}, nil
}
