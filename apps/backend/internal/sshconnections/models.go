// Package sshconnections owns managed SSH identities, trusted routes, and enrollment.
package sshconnections

import "errors"

var (
	ErrDisabled     = errors.New("ssh_connection_manager_disabled")
	ErrForbidden    = errors.New("ssh_management_forbidden")
	ErrNotFound     = errors.New("ssh_resource_not_found")
	ErrConflict     = errors.New("ssh_resource_changed")
	ErrInUse        = errors.New("ssh_resource_in_use")
	ErrInvalidRoute = errors.New("ssh_invalid_route")
)

// Identity contains public metadata only. Credentials never enter this model.
type Identity struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Kind        string `json:"kind"`
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
	Reference   string `json:"reference,omitempty"`
	Generation  string `json:"generation"`
	Version     int64  `json:"version"`
}

// Connection stores a resolved endpoint. An alias is display-only after resolution.
type Connection struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Host              string `json:"host"`
	Port              int    `json:"port"`
	User              string `json:"user"`
	Alias             string `json:"alias,omitempty"`
	IdentityID        string `json:"identity_id"`
	Fingerprint       string `json:"fingerprint"`
	PublicKey         string `json:"public_key"`
	BastionID         string `json:"bastion_id,omitempty"`
	Version           int64  `json:"version"`
	Revision          int64  `json:"revision"`
	VerifiedRoute     string `json:"verified_route,omitempty"`
	ImportedPending   bool   `json:"imported_pending,omitempty"`
	KnownHostsPending bool   `json:"known_hosts_pending,omitempty"`
	RepairReason      string `json:"repair_reason,omitempty"`
}

const (
	executorKindSSH = "ssh"
	outcomeFailed   = "failed"
	outcomeVerified = "verified"
	outcomeUnknown  = "outcome_unknown"
)
