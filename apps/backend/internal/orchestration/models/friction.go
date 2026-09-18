package models

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

type FrictionCause struct {
	Origin    string
	Operation string
	Reason    string
	Cause     string
}

// Friction carries typed provenance, never prompt bodies or arbitrary provider
// error text. AccountRevision is a configuration fingerprint, not a credential.
type Friction struct {
	Cause           string    `json:"cause" db:"cause"`
	ID              string    `json:"id" db:"id"`
	BindingID       string    `json:"binding_id" db:"binding_id"`
	WorkspaceID     string    `json:"workspace_id" db:"workspace_id"`
	TaskID          string    `json:"task_id" db:"task_id"`
	SessionID       string    `json:"session_id" db:"session_id"`
	OccurrenceID    string    `json:"source_id" db:"occurrence_id"`
	ProfileID       string    `json:"profile_id" db:"profile_id"`
	AccountRevision string    `json:"account_revision" db:"account_revision"`
	Origin          string    `json:"origin" db:"origin"`
	Operation       string    `json:"operation" db:"operation"`
	Reason          string    `json:"reason" db:"reason"`
	PolicyVersion   string    `json:"policy_version" db:"policy_version"`
	Fingerprint     string    `json:"fingerprint" db:"fingerprint"`
	Outcome         string    `json:"outcome" db:"outcome"`
	ObservedAt      time.Time `json:"observed_at" db:"observed_at"`
}

func (f Friction) Validate() error {
	for _, id := range []string{f.TaskID, f.OccurrenceID, f.ProfileID, f.AccountRevision, f.PolicyVersion} {
		if id == "" || len(id) > 200 || strings.ContainsAny(id, "\x00\r\n\t ") {
			return fmt.Errorf("friction requires bounded native identities")
		}
	}
	if len(f.SessionID) > 200 || (f.SessionID == "" && f.Operation != "launch") || strings.ContainsAny(f.SessionID, "\x00\r\n\t ") {
		return fmt.Errorf("invalid native session identity")
	}
	if f.Cause != "" && !slices.Contains(frictionCauses, f.Cause) {
		return fmt.Errorf("friction requires a known normalized cause")
	}
	if !slices.Contains([]string{"native", "provider", "configuration", "authentication", "unknown"}, f.Origin) ||
		!slices.Contains([]string{"permission", "question", "launch", "tool", "delivery"}, f.Operation) ||
		!slices.Contains([]string{"denied_authority", "pending_approval", "missing_capability", "authentication_required", "expired_authentication", "policy_boundary", "transport_failure", "suspected_classifier_error", "task_defect"}, f.Reason) ||
		!slices.Contains([]string{"blocked", "succeeded"}, f.Outcome) {
		return fmt.Errorf("friction requires a known origin, operation, reason and outcome")
	}
	return nil
}

func (f Friction) ScopeFingerprint() string {
	raw, _ := json.Marshal([]string{f.WorkspaceID, f.ProfileID, f.AccountRevision, f.Origin, f.Operation, f.Reason, f.Cause, f.PolicyVersion})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

var frictionCauses = []string{"question_request", "permission_request", "question_rejected", "permission_rejected", "permission_denied_by_user", "auth_required", "authentication_required", "authentication_expired", "token_expired", "missing_credentials", "provider_not_configured", "subscription_required", "model_unavailable", "network_unavailable", "provider_unavailable", "provider_overloaded", "rate_limited", "agent_transport_lost", "task_error", "repo_error", "policy_boundary", "suspected_classifier_error", "owner_report"}

type ImprovementCandidate struct {
	Cause           string     `json:"cause" db:"cause"`
	ID              string     `json:"id" db:"id"`
	BindingID       string     `json:"binding_id" db:"binding_id"`
	WorkspaceID     string     `json:"workspace_id" db:"workspace_id"`
	Fingerprint     string     `json:"fingerprint" db:"fingerprint"`
	ProfileID       string     `json:"profile_id" db:"profile_id"`
	AccountRevision string     `json:"account_revision" db:"account_revision"`
	Origin          string     `json:"origin" db:"origin"`
	Operation       string     `json:"operation" db:"operation"`
	Reason          string     `json:"reason" db:"reason"`
	PolicyVersion   string     `json:"policy_version" db:"policy_version"`
	State           string     `json:"state" db:"state"`
	Revision        int64      `json:"revision" db:"revision"`
	IncidentCount   int        `json:"incident_count" db:"incident_count"`
	TaskCount       int        `json:"task_count" db:"task_count"`
	RepairTaskID    string     `json:"repair_task_id" db:"repair_task_id"`
	ObjectiveID     string     `json:"objective_id" db:"objective_id"`
	CommitOID       string     `json:"commit_oid" db:"commit_oid"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
	PreparedAt      *time.Time `json:"prepared_at,omitempty" db:"prepared_at"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty" db:"resolved_at"`
}

type ImprovementReview struct {
	CandidateID  string    `json:"candidate_id" db:"candidate_id"`
	OwnerUserID  string    `json:"owner_user_id" db:"owner_user_id"`
	State        string    `json:"state" db:"state"`
	Evidence     Evidence  `json:"evidence" db:"-"`
	EvidenceJSON string    `json:"-" db:"evidence_json"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}
