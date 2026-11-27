package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

const humanInteractionReceiptTTL = 5 * time.Minute
const humanInteractionReceiptLimit = 10000

var (
	ErrHumanInteractionResponseInvalid     = errors.New("human interaction response is invalid")
	ErrHumanInteractionNotPending          = errors.New("interaction is not pending")
	ErrHumanInteractionWorkspaceMismatch   = errors.New("interaction workspace mismatch")
	ErrHumanInteractionVersionChanged      = errors.New("interaction version changed")
	ErrHumanInteractionResponseUnavailable = errors.New("human interaction response service unavailable")
)

type HumanInteractionResponse struct {
	Kind      string
	OptionID  string
	Cancelled bool
	Answers   []pluginsdk.ClarificationAnswer
}

type HumanInteractionResponseReceipt struct {
	ID, InteractionID, ResourceVersion, ExpiresAt string
}

type humanInteractionResponseReceipt struct {
	HumanActor, WorkspaceID, InteractionID, ResourceVersion string
	Kind, PayloadDigest                                     string
	ExpiresAt                                               time.Time
	// Delegation is set on a receipt minted for a linked Kandev user. Only the
	// installation that requested it can consume it.
	Delegation *receiptDelegation
}

type receiptDelegation struct {
	InstallationID, LinkID, Provider, Subject, UserDisplayName string
}

type humanInteractionResponseReceiptStore struct {
	mu      sync.Mutex
	entries map[string]humanInteractionResponseReceipt
	now     func() time.Time
}

func newHumanInteractionResponseReceiptStore() *humanInteractionResponseReceiptStore {
	return &humanInteractionResponseReceiptStore{entries: make(map[string]humanInteractionResponseReceipt), now: time.Now}
}

func exactHumanResponseDigest(kind string, in HumanInteractionResponse) string {
	canonical := struct {
		Kind      string                          `json:"kind"`
		OptionID  string                          `json:"option_id,omitempty"`
		Cancelled bool                            `json:"cancelled,omitempty"`
		Answers   []pluginsdk.ClarificationAnswer `json:"answers,omitempty"`
	}{Kind: kind, OptionID: in.OptionID, Cancelled: in.Cancelled, Answers: in.Answers}
	encoded, _ := json.Marshal(canonical)
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func (s *Service) IssueHumanInteractionResponseReceipt(
	ctx context.Context,
	humanActor, workspaceID, interactionID, expectedResourceVersion string,
	response HumanInteractionResponse,
) (HumanInteractionResponseReceipt, error) {
	if humanActor == "" || !isBoundedApprovalIdentifier(workspaceID) || !isBoundedApprovalIdentifier(interactionID) ||
		expectedResourceVersion == "" || (response.Kind != string(taskmodels.InteractionKindPermission) && response.Kind != string(taskmodels.InteractionKindClarification)) {
		return HumanInteractionResponseReceipt{}, ErrHumanInteractionResponseInvalid
	}
	if s.humanInteractionReceipts == nil {
		return HumanInteractionResponseReceipt{}, ErrHumanInteractionResponseUnavailable
	}
	version, err := s.pendingInteractionVersion(ctx, workspaceID, interactionID, expectedResourceVersion, response.Kind)
	if err != nil {
		return HumanInteractionResponseReceipt{}, err
	}
	return s.humanInteractionReceipts.issue(humanActor, workspaceID, interactionID, version, response, nil)
}

// pendingInteractionVersion verifies that an interaction of kind is pending
// in workspaceID at the observed version and returns that version.
func (s *Service) pendingInteractionVersion(ctx context.Context, workspaceID, interactionID, expectedResourceVersion, kind string) (string, error) {
	source := s.interactionData
	if source == nil || s.taskData == nil {
		return "", ErrHumanInteractionResponseUnavailable
	}
	interaction, err := source.GetInteraction(ctx, interactionID)
	if err != nil {
		return "", err
	}
	if interaction == nil || interaction.Status != taskmodels.InteractionStatusPending || string(interaction.Kind) != kind {
		return "", ErrHumanInteractionNotPending
	}
	task, err := s.taskData.GetTask(ctx, interaction.TaskID)
	if err != nil || task == nil || task.WorkspaceID != workspaceID {
		return "", ErrHumanInteractionWorkspaceMismatch
	}
	version := digestPublicValue(interactionModelToDTO(interaction))
	if version != expectedResourceVersion {
		return "", ErrHumanInteractionVersionChanged
	}
	return version, nil
}

func (s *humanInteractionResponseReceiptStore) issue(
	humanActor, workspaceID, interactionID, version string, response HumanInteractionResponse, delegation *receiptDelegation,
) (HumanInteractionResponseReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UTC()
	for id, record := range s.entries {
		if !record.ExpiresAt.After(now) {
			delete(s.entries, id)
		}
	}
	if len(s.entries) >= humanInteractionReceiptLimit {
		return HumanInteractionResponseReceipt{}, ErrHumanInteractionResponseUnavailable
	}
	id := uuid.NewString()
	expires := now.Add(humanInteractionReceiptTTL)
	s.entries[id] = humanInteractionResponseReceipt{
		HumanActor: humanActor, WorkspaceID: workspaceID, InteractionID: interactionID,
		ResourceVersion: version, Kind: response.Kind, PayloadDigest: exactHumanResponseDigest(response.Kind, response),
		ExpiresAt: expires, Delegation: delegation,
	}
	return HumanInteractionResponseReceipt{ID: id, InteractionID: interactionID, ResourceVersion: version, ExpiresAt: expires.Format(time.RFC3339Nano)}, nil
}

// discard removes a receipt that was minted but could not be recorded.
func (s *humanInteractionResponseReceiptStore) discard(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, id)
}

// consume removes and returns a receipt that matches the exact command. A
// delegated receipt presented by another installation is removed and
// rejected.
func (s *humanInteractionResponseReceiptStore) consume(
	id, installationID, workspaceID, interactionID, version, kind string, response HumanInteractionResponse,
) (humanInteractionResponseReceipt, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, found := s.entries[id]
	if !found {
		return humanInteractionResponseReceipt{}, false
	}
	if !record.ExpiresAt.After(s.now()) || (record.Delegation != nil && record.Delegation.InstallationID != installationID) {
		delete(s.entries, id)
		return humanInteractionResponseReceipt{}, false
	}
	if record.WorkspaceID != workspaceID || record.InteractionID != interactionID ||
		record.ResourceVersion != version || record.Kind != kind || record.PayloadDigest != exactHumanResponseDigest(kind, response) {
		return humanInteractionResponseReceipt{}, false
	}
	delete(s.entries, id)
	return record, true
}
