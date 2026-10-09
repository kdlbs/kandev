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
	"github.com/kandev/kandev/pkg/pluginsdk"
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

//nolint:cyclop // Receipt admission binds the human response, interaction version, and expiry.
func (s *Service) IssueHumanInteractionResponseReceipt(
	ctx context.Context,
	humanActor, workspaceID, interactionID, expectedResourceVersion string,
	response HumanInteractionResponse,
) (HumanInteractionResponseReceipt, error) {
	if humanActor == "" || !isBoundedApprovalIdentifier(workspaceID) || !isBoundedApprovalIdentifier(interactionID) ||
		expectedResourceVersion == "" || (response.Kind != string(taskmodels.InteractionKindPermission) && response.Kind != string(taskmodels.InteractionKindClarification)) {
		return HumanInteractionResponseReceipt{}, ErrHumanInteractionResponseInvalid
	}
	source := s.interactionData
	if source == nil || s.taskData == nil || s.humanInteractionReceipts == nil {
		return HumanInteractionResponseReceipt{}, ErrHumanInteractionResponseUnavailable
	}
	interaction, err := source.GetInteraction(ctx, interactionID)
	if err != nil {
		return HumanInteractionResponseReceipt{}, err
	}
	if interaction == nil || interaction.Status != taskmodels.InteractionStatusPending || string(interaction.Kind) != response.Kind {
		return HumanInteractionResponseReceipt{}, ErrHumanInteractionNotPending
	}
	task, err := s.taskData.GetTask(ctx, interaction.TaskID)
	if err != nil || task == nil || task.WorkspaceID != workspaceID {
		return HumanInteractionResponseReceipt{}, ErrHumanInteractionWorkspaceMismatch
	}
	version := digestPublicValue(interactionModelToDTO(interaction))
	if version != expectedResourceVersion {
		return HumanInteractionResponseReceipt{}, ErrHumanInteractionVersionChanged
	}
	return s.humanInteractionReceipts.issue(humanActor, workspaceID, interactionID, version, response)
}

func (s *humanInteractionResponseReceiptStore) issue(
	humanActor, workspaceID, interactionID, version string, response HumanInteractionResponse,
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
		ExpiresAt: expires,
	}
	return HumanInteractionResponseReceipt{ID: id, InteractionID: interactionID, ResourceVersion: version, ExpiresAt: expires.Format(time.RFC3339Nano)}, nil
}

func (s *humanInteractionResponseReceiptStore) consume(
	id, workspaceID, interactionID, version, kind string, response HumanInteractionResponse,
) (humanInteractionResponseReceipt, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, found := s.entries[id]
	if !found {
		return humanInteractionResponseReceipt{}, false
	}
	if !record.ExpiresAt.After(s.now()) {
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
