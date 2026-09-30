package messagequeue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	// ManagedInputOriginHuman marks caller-authored conversation input.
	ManagedInputOriginHuman ManagedInputOrigin = "human"
	// ManagedInputOriginAutomation marks explicit automation input.
	ManagedInputOriginAutomation ManagedInputOrigin = "automation"
	// ManagedInputOriginPeriodic marks recurring automation input. Only this
	// origin may use keyed pending-input coalescing.
	ManagedInputOriginPeriodic ManagedInputOrigin = "periodic"
	// ManagedInputOriginInteraction marks answers and other interaction input.
	ManagedInputOriginInteraction ManagedInputOrigin = "interaction"

	// ManagedInputStateAccepted means the input remains in the shared FIFO.
	ManagedInputStateAccepted ManagedInputState = "accepted"
	// ManagedInputStateRunning means an exact turn and execution own the input.
	ManagedInputStateRunning ManagedInputState = "running"
	// ManagedInputStateCompleted is a confirmed successful terminal result.
	ManagedInputStateCompleted ManagedInputState = "completed"
	// ManagedInputStateFailed is a confirmed failed terminal result.
	ManagedInputStateFailed ManagedInputState = "failed"
	// ManagedInputStateCancelled means accepted work was removed before start.
	ManagedInputStateCancelled ManagedInputState = "cancelled"
	// ManagedInputStateUncertain means execution may have produced an unobserved effect.
	ManagedInputStateUncertain ManagedInputState = "uncertain"
	// ManagedInputStateSuperseded means a periodic input replaced this pending input.
	ManagedInputStateSuperseded ManagedInputState = "superseded"

	// MetadataManagedInput marks a queue row owned by the managed-input API.
	MetadataManagedInput = "managed_input"
	// MetadataManagedInputID carries the host-minted managed input identity on a queue row.
	MetadataManagedInputID            = "managed_input_id"
	metadataManagedInputOccurrenceKey = "managed_input_occurrence_key"
	metadataManagedInputOrigin        = "managed_input_origin"
	metadataManagedInputDigest        = "managed_input_payload_digest"
	metadataManagedInputRevision      = "managed_input_conversation_revision"
)

// ManagedInputOrigin describes where an input came from. Non-periodic origins
// are always admitted as distinct FIFO entries, even when a coalesce key is
// supplied.
type ManagedInputOrigin string

// ManagedInputState is the durable outcome of an accepted managed input.
type ManagedInputState string

// ManagedInputRequest is one idempotent input admission. ID is host-minted;
// OccurrenceKey is the retry identity scoped to the exact task session.
type ManagedInputRequest struct {
	ID                   string
	OccurrenceKey        string
	PayloadDigest        string
	Payload              string
	Origin               ManagedInputOrigin
	CoalesceKey          string
	ConversationRevision int64
}

// ManagedInputReceipt records admission and execution state independently of
// the prompt row. Sequence is assigned transactionally for each new receipt;
// coalescing moves the replacement to the FIFO tail so sequence and delivery
// order remain consistent.
type ManagedInputReceipt struct {
	ID                   string             `json:"id"`
	OccurrenceKey        string             `json:"occurrence_key"`
	TaskID               string             `json:"task_id"`
	SessionID            string             `json:"session_id"`
	SessionIncarnationID string             `json:"session_incarnation_id"`
	PayloadDigest        string             `json:"payload_digest"`
	Payload              string             `json:"payload"`
	Origin               ManagedInputOrigin `json:"origin"`
	CoalesceKey          string             `json:"coalesce_key,omitempty"`
	Sequence             int64              `json:"sequence"`
	ConversationRevision int64              `json:"conversation_revision"`
	State                ManagedInputState  `json:"state"`
	CreatedAt            time.Time          `json:"created_at"`
	UpdatedAt            time.Time          `json:"updated_at"`
	TurnID               string             `json:"turn_id,omitempty"`
	ExecutionID          string             `json:"execution_id,omitempty"`
	SupersededBy         string             `json:"superseded_by,omitempty"`
	Outcome              string             `json:"outcome,omitempty"`
	fingerprint          string
}

type managedInputScope struct {
	TaskID               string
	SessionID            string
	SessionIncarnationID string
}

type managedInputIDKey struct {
	Scope   managedInputScope
	InputID string
}

type managedInputOccurrenceKey struct {
	Scope         managedInputScope
	OccurrenceKey string
}

// ManagedInputStorage is the managed-conversation receipt API implemented by
// the queue repositories. It is separate from Repository so queue test fakes
// and legacy callers do not need to implement the managed-input contract.
type ManagedInputStorage interface {
	// AdmitManagedInput atomically persists the receipt and shared FIFO row.
	AdmitManagedInput(
		context.Context,
		QueueSessionIdentity,
		ManagedInputRequest,
		int,
	) (ManagedInputReceipt, bool, error)
	// GetManagedInput reads one receipt within the exact task-session identity.
	GetManagedInput(context.Context, QueueSessionIdentity, string) (ManagedInputReceipt, error)
	// GetManagedInputByExecution finds the receipt linked to the exact turn and execution.
	GetManagedInputByExecution(
		context.Context,
		QueueSessionIdentity,
		string,
		string,
	) (ManagedInputReceipt, error)
	// ListManagedInputs returns scoped receipts in FIFO sequence order.
	ListManagedInputs(context.Context, QueueSessionIdentity) ([]ManagedInputReceipt, error)
	// CancelManagedInput cancels accepted work; running work remains unchanged.
	CancelManagedInput(
		context.Context,
		QueueSessionIdentity,
		string,
	) (ManagedInputReceipt, bool, error)
	// MarkManagedInputRunning claims the FIFO head for one exact execution.
	MarkManagedInputRunning(
		context.Context,
		QueueSessionIdentity,
		string,
		string,
		string,
	) (ManagedInputReceipt, bool, error)
	// SettleManagedInput records a terminal, uncertain, or confirmed-stop result for that execution.
	SettleManagedInput(
		context.Context,
		QueueSessionIdentity,
		string,
		string,
		string,
		ManagedInputState,
		string,
	) (ManagedInputReceipt, bool, error)
}

var (
	// ErrManagedInputNotPending means no pending FIFO row can be claimed.
	ErrManagedInputNotPending = errors.New("managed input is not pending")
	// ErrManagedInputNotHead means an earlier queue row must be handled first.
	ErrManagedInputNotHead = errors.New("managed input is not the FIFO head")
	// ErrManagedInputTransition means state or execution identity does not match.
	ErrManagedInputTransition = errors.New("managed input state transition conflict")
	// ErrManagedInputBusy means another managed input already owns the conversation turn.
	ErrManagedInputBusy = errors.New("managed conversation already has a running input")
)

func managedInputScopeFor(identity QueueSessionIdentity) managedInputScope {
	return managedInputScope(identity)
}

func validateManagedInputRequest(identity QueueSessionIdentity, request ManagedInputRequest) error {
	if identity.TaskID == "" || identity.SessionID == "" || identity.SessionIncarnationID == "" {
		return ErrSessionIdentityMismatch
	}
	if request.ID == "" || request.OccurrenceKey == "" ||
		len(request.ID) > MaxQueueAdmissionIDLength || len(request.OccurrenceKey) > MaxQueueAdmissionIDLength {
		return errors.New("managed input identity is invalid")
	}
	if request.PayloadDigest == "" || len(request.PayloadDigest) > 128 || len(request.CoalesceKey) > 256 {
		return errors.New("managed input payload digest or coalesce key is invalid")
	}
	switch request.Origin {
	case ManagedInputOriginHuman, ManagedInputOriginAutomation,
		ManagedInputOriginPeriodic, ManagedInputOriginInteraction:
	default:
		return errors.New("managed input origin is invalid")
	}
	if request.ConversationRevision < 0 {
		return errors.New("managed input conversation revision is invalid")
	}
	return nil
}

func managedInputFingerprint(identity QueueSessionIdentity, request ManagedInputRequest) (string, error) {
	canonical := struct {
		TaskID               string             `json:"task_id"`
		SessionID            string             `json:"session_id"`
		SessionIncarnationID string             `json:"session_incarnation_id"`
		OccurrenceKey        string             `json:"occurrence_key"`
		PayloadDigest        string             `json:"payload_digest"`
		Payload              string             `json:"payload"`
		Origin               ManagedInputOrigin `json:"origin"`
		CoalesceKey          string             `json:"coalesce_key"`
		ConversationRevision int64              `json:"conversation_revision"`
	}{
		TaskID: identity.TaskID, SessionID: identity.SessionID,
		SessionIncarnationID: identity.SessionIncarnationID,
		OccurrenceKey:        request.OccurrenceKey, PayloadDigest: request.PayloadDigest,
		Payload: request.Payload, Origin: request.Origin, CoalesceKey: request.CoalesceKey,
		ConversationRevision: request.ConversationRevision,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("encode managed input fingerprint: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func managedInputQueueMessage(identity QueueSessionIdentity, request ManagedInputRequest) *QueuedMessage {
	return &QueuedMessage{
		ID: request.ID, TaskID: identity.TaskID, SessionID: identity.SessionID,
		Content: request.Payload, QueuedBy: QueuedByServer,
		Metadata: map[string]interface{}{
			MetadataManagedInput:              true,
			MetadataManagedInputID:            request.ID,
			metadataManagedInputOccurrenceKey: request.OccurrenceKey,
			metadataManagedInputOrigin:        string(request.Origin),
			metadataManagedInputDigest:        request.PayloadDigest,
			metadataManagedInputRevision:      request.ConversationRevision,
			"managed_input_coalesce_key":      request.CoalesceKey,
		},
		QueuedAt: time.Now().UTC(),
	}
}

func managedInputReceiptFromRequest(
	identity QueueSessionIdentity,
	request ManagedInputRequest,
	sequence int64,
	now time.Time,
) ManagedInputReceipt {
	return ManagedInputReceipt{
		ID: request.ID, OccurrenceKey: request.OccurrenceKey,
		TaskID: identity.TaskID, SessionID: identity.SessionID,
		SessionIncarnationID: identity.SessionIncarnationID,
		PayloadDigest:        request.PayloadDigest, Payload: request.Payload,
		Origin: request.Origin, CoalesceKey: request.CoalesceKey,
		Sequence: sequence, ConversationRevision: request.ConversationRevision,
		State: ManagedInputStateAccepted, CreatedAt: now, UpdatedAt: now,
	}
}

func managedInputIsTerminal(state ManagedInputState) bool {
	return state == ManagedInputStateCompleted || state == ManagedInputStateFailed ||
		state == ManagedInputStateUncertain || state == ManagedInputStateCancelled
}

func validateManagedInputSettlement(turnID, executionID string, state ManagedInputState, outcome string) error {
	if !managedInputIsTerminal(state) {
		return errors.New("managed input settlement state is invalid")
	}
	if len(outcome) > 2048 {
		return errors.New("managed input outcome is too long")
	}
	if (turnID == "") != (executionID == "") {
		return errors.New("managed input turn and execution identity must be supplied together")
	}
	if turnID == "" && state != ManagedInputStateUncertain {
		return errors.New("only uncertain managed input settlement may omit execution identity")
	}
	return nil
}

func managedInputSort(receipts []ManagedInputReceipt) {
	sort.Slice(receipts, func(i, j int) bool {
		if receipts[i].Sequence != receipts[j].Sequence {
			return receipts[i].Sequence < receipts[j].Sequence
		}
		if !receipts[i].CreatedAt.Equal(receipts[j].CreatedAt) {
			return receipts[i].CreatedAt.Before(receipts[j].CreatedAt)
		}
		return receipts[i].ID < receipts[j].ID
	})
}

func managedInputCanCoalesce(request ManagedInputRequest) bool {
	return request.Origin == ManagedInputOriginPeriodic && strings.TrimSpace(request.CoalesceKey) != ""
}
