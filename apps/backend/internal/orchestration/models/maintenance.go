package models

import (
	"fmt"
	"path"
	"slices"
	"strings"
	"time"
)

// MaintenanceScope grants closed local preparation operations. It cannot name a
// shell, remote publisher, live checkout or provider permission policy.
type MaintenanceScope struct {
	RepositoryID   string   `json:"repository_id"`
	WorkflowID     string   `json:"workflow_id"`
	WorkflowStepID string   `json:"workflow_step_id"`
	ProfileID      string   `json:"profile_id"`
	Files          []string `json:"files"`
	Actions        []string `json:"actions"`
	Image          string   `json:"image"`
	Positive       []string `json:"positive_check"`
	Negative       []string `json:"negative_check"`
}

type MaintenanceGrant struct {
	CandidateID       string           `json:"candidate_id" db:"candidate_id"`
	BindingID         string           `json:"binding_id" db:"binding_id"`
	OwnerUserID       string           `json:"owner_user_id" db:"owner_user_id"`
	WorkspaceID       string           `json:"workspace_id" db:"workspace_id"`
	BindingVersion    int64            `json:"binding_version" db:"binding_version"`
	AuthorityRevision string           `json:"authority_revision" db:"authority_revision"`
	ProfileRevision   string           `json:"profile_revision" db:"profile_revision"`
	Revision          int64            `json:"revision" db:"revision"`
	Scope             MaintenanceScope `json:"scope" db:"-"`
	ScopeJSON         string           `json:"-" db:"scope_json"`
	BaseOID           string           `json:"base_oid" db:"base_oid"`
	ExpiresAt         time.Time        `json:"expires_at" db:"expires_at"`
	RevokedAt         *time.Time       `json:"revoked_at,omitempty" db:"revoked_at"`
	CreatedAt         time.Time        `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at" db:"updated_at"`
}

type MaintenanceFile struct {
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Content string `json:"content"`
}

type MaintenanceCheck struct {
	Kind       string `json:"kind"`
	ExitCode   int    `json:"exit_code"`
	OutputHash string `json:"output_hash"`
	Summary    string `json:"summary"`
}

type MaintenanceValidation struct {
	TreeOID       string             `json:"tree_oid"`
	GrantRevision int64              `json:"grant_revision"`
	Checks        []MaintenanceCheck `json:"checks"`
	Passed        bool               `json:"passed"`
}

type MaintenanceArtifact struct {
	BaseOID   string `json:"base_oid"`
	TreeOID   string `json:"tree_oid"`
	CommitOID string `json:"commit_oid"`
}

type MaintenanceReviewArtifact struct {
	MaintenanceArtifact
	Patch       string                 `json:"patch"`
	PatchSHA256 string                 `json:"patch_sha256"`
	Validation  *MaintenanceValidation `json:"validation,omitempty"`
}

type MaintenanceOption struct {
	ID         string `json:"id"`
	ResourceID string `json:"resource_id"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	WorkflowID string `json:"workflow_id,omitempty"`
}

type MaintenanceSuccess struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"task_id"`
	TaskTitle   string    `json:"task_title"`
	SessionID   string    `json:"session_id"`
	SourceID    string    `json:"source_id"`
	CompletedAt time.Time `json:"completed_at"`
}

func (s MaintenanceScope) Validate() error {
	for _, id := range []string{s.RepositoryID, s.WorkflowID, s.WorkflowStepID, s.ProfileID, s.Image} {
		if id == "" || len(id) > 200 || strings.ContainsAny(id, "\x00\r\n") {
			return fmt.Errorf("maintenance requires explicit bounded resource selections")
		}
	}
	if len(s.Files) == 0 || len(s.Files) > 32 || len(s.Actions) == 0 || len(s.Actions) > 4 {
		return fmt.Errorf("maintenance requires a narrow file and action scope")
	}
	seen := map[string]bool{}
	for _, file := range s.Files {
		if !MaintenanceFileAllowed(file) || seen[file] {
			return fmt.Errorf("maintenance file is protected or outside the supported scope")
		}
		seen[file] = true
	}
	return s.validateActions()
}

func (s MaintenanceScope) validateActions() error {
	seen := map[string]bool{}
	for _, action := range s.Actions {
		if !slices.Contains([]string{"read", "patch", "test", "commit"}, action) || seen[action] {
			return fmt.Errorf("maintenance grants only read, patch, test and local commit")
		}
		seen[action] = true
	}
	if !validMaintenanceCommand(s.Positive) || !validMaintenanceCommand(s.Negative) {
		return fmt.Errorf("explicit positive and negative validation commands are required")
	}
	return nil
}

func validMaintenanceCommand(argv []string) bool {
	if len(argv) == 0 || len(argv) > 32 || argv[0] == "" || strings.HasPrefix(argv[0], "-") {
		return false
	}
	for _, arg := range argv {
		if len(arg) > 2048 || strings.ContainsAny(arg, "\x00\r\n") {
			return false
		}
	}
	return true
}

// Policy/configuration repair is intentionally unsupported. The check is in
// addition to isolation and the human's exact file allowlist, not an authority
// classifier for arbitrary code semantics.
func MaintenanceFileAllowed(file string) bool {
	if file == "" || len(file) > 240 || path.IsAbs(file) || path.Clean(file) != file || file == "." ||
		strings.HasPrefix(file, "../") || strings.ContainsAny(file, "\\*?[]{}\x00\r\n:") {
		return false
	}
	lower := strings.ToLower(file)
	for _, protected := range []string{".git", ".claude", ".codex", ".agents", ".env", "agents.md", "claude.md", "node_modules/", "vendor/", "permission", "allowlist", "approval", "auth", "credential", "policy", "security", "internal/agent", "internal/mcp/", "internal/orchestrat", "internal/backendapp/", "internal/runtimeflags/", "internal/common/config/"} {
		if strings.Contains(lower, protected) {
			return false
		}
	}
	return true
}
