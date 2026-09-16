package plugins

import "time"

type CapabilityApprovalDTO struct {
	InstallationID     string    `json:"installation_id"`
	WorkspaceID        string    `json:"workspace_id"`
	Revision           uint64    `json:"revision"`
	ManifestDigest     string    `json:"manifest_digest"`
	CapabilityIDs      []string  `json:"capability_ids"`
	State              string    `json:"state"`
	HumanActor         string    `json:"human_actor"`
	HumanPolicyVersion string    `json:"human_policy_version"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type CapabilityApprovalEventDTO struct {
	AuditID        string    `json:"audit_id"`
	InstallationID string    `json:"installation_id"`
	WorkspaceID    string    `json:"workspace_id"`
	BeforeRevision uint64    `json:"before_revision"`
	AfterRevision  uint64    `json:"after_revision"`
	BeforeDigest   string    `json:"before_digest"`
	AfterDigest    string    `json:"after_digest"`
	Actor          string    `json:"actor"`
	Reason         string    `json:"reason"`
	Type           string    `json:"type"`
	ObservedAt     time.Time `json:"observed_at"`
}

func approvalDTOFromCurrent(current CapabilityApproval) CapabilityApprovalDTO {
	return CapabilityApprovalDTO{
		InstallationID:     current.InstallationID,
		WorkspaceID:        current.WorkspaceID,
		Revision:           current.Revision,
		ManifestDigest:     current.ManifestDigest,
		CapabilityIDs:      append([]string{}, current.CapabilityIDs...),
		State:              string(current.State),
		HumanActor:         current.HumanActor,
		HumanPolicyVersion: current.HumanPolicyVersion,
		CreatedAt:          current.CreatedAt,
		UpdatedAt:          current.UpdatedAt,
	}
}

func approvalEventDTOFromCurrent(event CapabilityApprovalEvent) CapabilityApprovalEventDTO {
	return CapabilityApprovalEventDTO{
		AuditID:        event.AuditID,
		InstallationID: event.InstallationID,
		WorkspaceID:    event.WorkspaceID,
		BeforeRevision: event.BeforeRevision,
		AfterRevision:  event.AfterRevision,
		BeforeDigest:   event.BeforeDigest,
		AfterDigest:    event.AfterDigest,
		Actor:          event.Actor,
		Reason:         event.Reason,
		Type:           string(event.Type),
		ObservedAt:     event.ObservedAt,
	}
}
