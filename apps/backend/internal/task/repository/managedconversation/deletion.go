package managedconversation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type DeleteRequest struct {
	Identity
	ExpectedRevision uint64    `json:"expected_revision"`
	TaskCreatedAt    time.Time `json:"task_created_at"`
	OperationID      string    `json:"operation_id"`
	PayloadDigest    string    `json:"payload_digest"`
	Ordinary         bool      `json:"ordinary,omitempty"`
}

const (
	DeleteReserved  = "reserved"
	DeletePrepared  = "prepared"
	DeleteCommitted = "deleted"
)

// DeleteClaim names one invocation, independently of command idempotency.
type DeleteClaim struct {
	DeleteRequest
	Version int    `json:"version"`
	JobID   string `json:"job_id"`
	Owner   string `json:"owner"`
	Phase   string `json:"phase"`
}

// DeletionRepository is mandatory for lifecycle deletion of retained rows.
type DeletionRepository interface {
	AdmitManagedDeletion(context.Context, DeleteRequest, string) (*DeleteClaim, error)
	InspectManagedDeletion(context.Context, DeleteRequest) (*DeleteClaim, *models.TaskResourceCleanupJob, error)
	PrepareManagedDeletion(context.Context, DeleteClaim, string) (*models.TaskResourceCleanupJob, error)
	FinalizeManagedDeletion(context.Context, DeleteClaim) (string, error)
	ReleaseManagedDeletion(context.Context, DeleteClaim) (bool, error)
	TransferManagedDeletionEnvironment(context.Context, DeleteClaim, string, string, int64, string) error
}

func DeleteOperationID(operation string) string {
	digest := sha256.Sum256([]byte(operation))
	return "managed-delete:" + hex.EncodeToString(digest[:])
}

func DeletionEnvelope(snapshot string) (*DeleteClaim, error) {
	if strings.TrimSpace(snapshot) == "" {
		return nil, nil
	}
	var value struct {
		ManagedDelete *DeleteClaim `json:"managed_delete"`
	}
	if err := json.Unmarshal([]byte(snapshot), &value); err != nil {
		return nil, err
	}
	return value.ManagedDelete, nil
}

func WithDeletionEnvelope(snapshot string, claim DeleteClaim) (string, error) {
	var value map[string]json.RawMessage
	if err := json.Unmarshal([]byte(snapshot), &value); err != nil {
		return "", err
	}
	if value == nil {
		value = make(map[string]json.RawMessage)
	}
	encoded, err := json.Marshal(claim)
	if err != nil {
		return "", err
	}
	value["managed_delete"] = encoded
	encoded, err = json.Marshal(value)
	return string(encoded), err
}
