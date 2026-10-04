// Package managedconversation defines native retained-conversation admission.
package managedconversation

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"

	"github.com/kandev/kandev/internal/task/models"
)

var (
	ErrNotFound    = errors.New("managed conversation not found")
	ErrRevision    = errors.New("managed conversation revision is stale")
	ErrBusy        = errors.New("managed conversation launch settings can change only while idle")
	ErrUnavailable = errors.New("managed conversation admission is unavailable")
)

const (
	OperationKey   = "kandev.last_exact_operation"
	PayloadKey     = "kandev.last_exact_payload"
	PromptKey      = "kandev.base_prompt"
	InstructionKey = "kandev.instruction_version"
)

type Identity struct {
	TaskID, InstallationID, WorkspaceID, InstanceKey string
}

type Configuration struct {
	AgentProfileID, ExecutorID, ExecutorProfileID  string
	BasePrompt, InstructionVersion, ManifestDigest string
	ApprovalRevision                               uint64
	AgentToolNames                                 []string
}

type EnsureRequest struct {
	Identity
	Configuration
	Task                       *models.Task
	PrimaryID                  string
	ExpectedRevision           uint64
	OperationID, PayloadDigest string
}

type StateKind uint8

const (
	PauseExact StateKind = iota
	PauseInstallation
	Invalidate
	Detach
)

type StateRequest struct {
	Identity
	Kind                       StateKind
	Paused                     bool
	ExpectedRevision           uint64
	OperationID, PayloadDigest string
	PrimaryID                  string
}

type Result struct {
	Task                       *models.Task
	Primary                    *models.TaskSession
	Created, Changed, Replayed bool
}

type Repository interface {
	EnsureManagedConversation(context.Context, EnsureRequest) (Result, error)
	ChangeManagedConversationState(context.Context, StateRequest) (Result, error)
}

func Revision(task *models.Task) uint64 {
	n, _ := strconv.ParseUint(models.StringFromAny(task.Metadata[models.MetaKeyManagedConversationRevision]), 10, 64)
	if n == 0 {
		return 1
	}
	return n
}

func Matches(task *models.Task, identity Identity) bool {
	m := task.Metadata
	return task.ID == identity.TaskID && task.WorkspaceID == identity.WorkspaceID &&
		m[models.MetaKeyManagedRetained] == true && m["kandev.ephemeral"] == true &&
		models.StringFromAny(m[models.MetaKeyManagedInstallationID]) == identity.InstallationID &&
		models.StringFromAny(m["kandev.workspace_id"]) == identity.WorkspaceID &&
		models.StringFromAny(m[models.MetaKeyManagedInstanceKey]) == identity.InstanceKey
}

func Replay(task *models.Task, operationID, digest string) bool {
	return operationID != "" && digest != "" && models.StringFromAny(task.Metadata[OperationKey]) == operationID &&
		models.StringFromAny(task.Metadata[PayloadKey]) == digest
}

func FromTask(task *models.Task) Configuration {
	m := task.Metadata
	approval, _ := strconv.ParseUint(models.StringFromAny(m[models.MetaKeyManagedApprovalRevision]), 10, 64)
	var tools []string
	encoded, _ := json.Marshal(m[models.MetaKeyManagedAgentToolNames])
	_ = json.Unmarshal(encoded, &tools)
	slices.Sort(tools)
	return Configuration{
		AgentProfileID:    models.StringFromAny(m[models.MetaKeyAgentProfileID]),
		ExecutorID:        models.StringFromAny(m[models.MetaKeyExecutorID]),
		ExecutorProfileID: models.StringFromAny(m[models.MetaKeyExecutorProfileID]),
		BasePrompt:        models.StringFromAny(m[PromptKey]), InstructionVersion: models.StringFromAny(m[InstructionKey]),
		ManifestDigest: models.StringFromAny(m[models.MetaKeyManagedManifestDigest]), ApprovalRevision: approval, AgentToolNames: tools,
	}
}

func (c Configuration) Equal(other Configuration) bool {
	return c.AgentProfileID == other.AgentProfileID && c.ExecutorID == other.ExecutorID &&
		c.ExecutorProfileID == other.ExecutorProfileID && c.BasePrompt == other.BasePrompt &&
		c.InstructionVersion == other.InstructionVersion && c.ManifestDigest == other.ManifestDigest &&
		c.ApprovalRevision == other.ApprovalRevision && slices.Equal(c.AgentToolNames, other.AgentToolNames)
}

func (c Configuration) Values() map[string]interface{} {
	return map[string]interface{}{
		models.MetaKeyAgentProfileID: c.AgentProfileID, models.MetaKeyExecutorID: c.ExecutorID,
		models.MetaKeyExecutorProfileID: c.ExecutorProfileID, PromptKey: c.BasePrompt, InstructionKey: c.InstructionVersion,
		models.MetaKeyManagedApprovalRevision: strconv.FormatUint(c.ApprovalRevision, 10),
		models.MetaKeyManagedManifestDigest:   c.ManifestDigest, models.MetaKeyManagedAgentToolNames: c.AgentToolNames,
		models.MetaKeyManagedPolicyInvalidated: false,
	}
}
