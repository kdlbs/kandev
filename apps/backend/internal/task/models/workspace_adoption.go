package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

var ErrWorkspaceAdoptionStale = errors.New("workspace adoption identity changed")

// WorkspaceAdoptionSelection is authoritative server state, not caller authority.
type WorkspaceAdoptionSelection struct {
	Snapshot                 WorkspaceRecoverySelectionSnapshot `json:"snapshot"`
	WorkspaceID              string                             `json:"workspace_id"`
	SessionIncarnation       string                             `json:"session_incarnation"`
	SessionWorkspacePath     string                             `json:"session_workspace_path"`
	AttachmentCheckoutBranch string                             `json:"attachment_checkout_branch"`
	AttachmentBaseBranch     string                             `json:"attachment_base_branch"`
	AttachmentID             string                             `json:"attachment_id"`
	AttachmentsDigest        string                             `json:"attachments_digest"`
}

type WorkspaceAdoptionProof struct {
	SourceHead       string            `json:"source_head"`
	RootMarkerSHA256 string            `json:"root_marker_sha256"`
	OriginalPath     string            `json:"original_path"`
	CandidatePath    string            `json:"candidate_path"`
	MarkerSHA256     string            `json:"marker_sha256"`
	MissingGitDir    string            `json:"missing_git_dir"`
	OriginSHA256     string            `json:"origin_sha256"`
	Branch           string            `json:"branch"`
	Head             string            `json:"head"`
	IndexSHA256      string            `json:"index_sha256"`
	Identities       map[string]string `json:"identities"`
}

type WorkspaceAdoptionPreview struct {
	Selection WorkspaceAdoptionSelection `json:"selection"`
	SlotID    string                     `json:"slot_id"`
	Proof     WorkspaceAdoptionProof     `json:"proof"`
	Token     string                     `json:"token"`
}

type WorkspaceAdoptionReceipt struct {
	OperationID string                   `json:"operation_id"`
	Preview     WorkspaceAdoptionPreview `json:"preview"`
	CreatedAt   time.Time                `json:"created_at"`
}

func WorkspaceAdoptionDigest(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Empty metadata has the same branch authority regardless of repository decoding.
func WorkspaceAdoptionMetadataDigest(metadata map[string]interface{}) string {
	if len(metadata) == 0 {
		metadata = map[string]interface{}{}
	}
	return WorkspaceAdoptionDigest(metadata)
}

func (p WorkspaceAdoptionPreview) Digest() string {
	p.Token = ""
	return WorkspaceAdoptionDigest(p)
}

// PublishedWorkspacePath projects a checkout working directory onto its replacement.
// Task-root working directories retain their original path.
func (p WorkspaceAdoptionPreview) PublishedWorkspacePath(path string) string {
	if path == p.Proof.OriginalPath {
		return p.Proof.CandidatePath
	}
	return path
}

func (p WorkspaceAdoptionPreview) Slot() (WorkspaceRecoveryInventorySlot, error) {
	for _, s := range p.Selection.Snapshot.Slots {
		if s.EnvironmentRepoID == p.SlotID {
			return s, nil
		}
	}
	return WorkspaceRecoveryInventorySlot{}, ErrWorkspaceAdoptionStale
}

// WorkspaceAdoptionAttachment captures all branch/order authority for CAS.
type WorkspaceAdoptionAttachment struct {
	ID             string `json:"id" db:"id"`
	TaskID         string `json:"task_id" db:"task_id"`
	RepositoryID   string `json:"repository_id" db:"repository_id"`
	CheckoutBranch string `json:"checkout_branch" db:"checkout_branch"`
	BaseBranch     string `json:"base_branch" db:"base_branch"`
	MetadataDigest string `json:"metadata_digest" db:"-"`
	Position       int    `json:"position" db:"position"`
}
