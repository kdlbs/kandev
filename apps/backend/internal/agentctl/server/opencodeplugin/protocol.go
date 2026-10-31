package opencodeplugin

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrAdmissionRevoked tells the host that a pre-journal fence invalidated the
// observation. It is not retryable under the same claim.
var ErrAdmissionRevoked = errors.New("opencode plugin admission was revoked")

// Lease is the launch-scoped identity shared with the private plugin host.
// The credential never leaves the mode-0600 bootstrap file.
type Lease struct {
	ID           string
	Credential   string
	ArtifactHash string
}

// NewLease creates a random launch identity.
func NewLease(artifactHash string) (Lease, error) {
	id, err := randomHex(16)
	if err != nil {
		return Lease{}, err
	}
	credential, err := randomHex(32)
	if err != nil {
		return Lease{}, err
	}
	return Lease{ID: id, Credential: credential, ArtifactHash: artifactHash}, nil
}

// Authorized reports whether the supplied bearer credential belongs to this
// lease. Comparison is constant time.
func (l Lease) Authorized(value string) bool {
	if len(value) != len(l.Credential) || value == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(value), []byte(l.Credential)) == 1
}

// BehaviorRef names one compiled behavior version.
type BehaviorRef struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

// Bootstrap is the private launch file the plugin reads through its options.
type Bootstrap struct {
	ProtocolVersion int           `json:"protocolVersion"`
	Endpoint        string        `json:"endpoint"`
	Credential      string        `json:"credential"`
	LeaseID         string        `json:"leaseId"`
	ArtifactHash    string        `json:"artifactHash"`
	Behaviors       []BehaviorRef `json:"behaviors"`
}

// Claim is Kandev's immutable association between one confirmed background
// launch and its native shell/session identity.
type Claim struct {
	ShellID          string `json:"shellId"`
	NativeSessionID  string `json:"nativeSessionId"`
	NativeGeneration uint64 `json:"nativeGeneration"`
	WorkID           string `json:"workId"`
	RunID            string `json:"runId"`
	SourceCallID     string `json:"sourceCallId"`
	Title            string `json:"title"`
	PromptGeneration uint64 `json:"promptGeneration"`
	SubmissionID     string `json:"submissionId"`
}

// Validate rejects malformed claims before they become authority.
func (c Claim) Validate() error {
	switch {
	case strings.TrimSpace(c.ShellID) == "":
		return errors.New("claim shell id is required")
	case strings.TrimSpace(c.NativeSessionID) == "":
		return errors.New("claim native session id is required")
	case c.NativeGeneration == 0:
		return errors.New("claim native generation is required")
	case strings.TrimSpace(c.WorkID) == "":
		return errors.New("claim work id is required")
	}
	return nil
}

// HelloRequest is sent by the plugin after it installs its native subscription.
type HelloRequest struct {
	ProtocolVersion int           `json:"protocolVersion"`
	LeaseID         string        `json:"leaseId"`
	ArtifactHash    string        `json:"artifactHash"`
	Runtime         RuntimeInfo   `json:"runtime"`
	Behaviors       []BehaviorRef `json:"behaviors"`
}

// RuntimeInfo identifies the actual runtime that loaded the plugin.
type RuntimeInfo struct {
	Family  string `json:"family"`
	Version string `json:"version"`
}

// HelloResponse reports the negotiated behavior versions and readiness.
type HelloResponse struct {
	ProtocolVersion int           `json:"protocolVersion"`
	LeaseID         string        `json:"leaseId"`
	Ready           bool          `json:"ready"`
	Behaviors       []BehaviorRef `json:"behaviors"`
}

// BehaviorConfig is the complete revisioned configuration for one behavior.
// Data is behavior-owned; the shared host never interprets it.
type BehaviorConfig struct {
	Version int             `json:"version"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// ConfigResponse carries complete configuration for every negotiated module.
type ConfigResponse struct {
	ProtocolVersion int                       `json:"protocolVersion"`
	LeaseID         string                    `json:"leaseId"`
	Revision        uint64                    `json:"revision"`
	Behaviors       map[string]BehaviorConfig `json:"behaviors"`
}

// ShellTerminalPayload is the only payload accepted by the shell module.
type ShellTerminalPayload struct {
	Kind        string `json:"kind"`
	ShellID     string `json:"shellId"`
	Status      string `json:"status"`
	ExitCode    *int   `json:"exitCode,omitempty"`
	StartedAt   *int64 `json:"startedAt,omitempty"`
	CompletedAt *int64 `json:"completedAt,omitempty"`
}

// ObservationEnvelope is one typed native observation.
type ObservationEnvelope struct {
	EventID          string          `json:"eventId"`
	BehaviorID       string          `json:"behaviorId"`
	BehaviorVersion  int             `json:"behaviorVersion"`
	Sequence         uint64          `json:"sequence"`
	NativeSessionID  string          `json:"nativeSessionId"`
	NativeGeneration uint64          `json:"nativeGeneration"`
	Payload          json.RawMessage `json:"payload"`
}

// EventRequest is a bounded event batch.
type EventRequest struct {
	ProtocolVersion int                   `json:"protocolVersion"`
	LeaseID         string                `json:"leaseId"`
	ProducerID      string                `json:"producerId"`
	Events          []ObservationEnvelope `json:"events"`
}

// EventRejection explains one event that had no effects.
type EventRejection struct {
	EventID string `json:"eventId"`
	Code    string `json:"code"`
}

// EventResponse acknowledges durable acceptance per stable event identity.
type EventResponse struct {
	Accepted []string         `json:"accepted"`
	Rejected []EventRejection `json:"rejected"`
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("request contains trailing content")
	}
	return nil
}

func randomHex(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("read launch randomness: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}
