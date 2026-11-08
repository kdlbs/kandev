// Package sshlaunch owns the immutable launch evidence a managed SSH agentctl
// controller self-reports during its authenticated bootstrap handshake.
//
// The receipt exists for one reason: the controller's session directory is
// disposable, and every other durable stop signal (pidfile, command line,
// workdir) can be reproduced by a replacement launch or a sibling session.
// Recording the controller's own process birth identity at launch is the only
// way to keep owner-bound authority after that directory disappears.
//
// Both the backend and the standalone agentctl helper import this package, so
// it must stay free of backend-only or server-only dependencies.
package sshlaunch

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/common/processidentity"
)

const (
	// Version is the receipt schema this build writes and accepts.
	Version = 1
	// MetadataKey is the executors_running.metadata key holding the receipt.
	MetadataKey = "ssh_agentctl_launch_receipt"
	// GenerationEnv carries the private per-attempt launch generation from the
	// backend to the managed agentctl child. It is an identity marker, never a
	// credential, and it is stripped from every agent subprocess environment.
	GenerationEnv = "KANDEV_INTERNAL_SSH_LAUNCH_GENERATION"

	generationBytes = 16
)

var (
	// ErrIncompleteReceipt means a field required to prove ownership is
	// missing. Callers must treat it as "no launch proof", never as a
	// partial match.
	ErrIncompleteReceipt = errors.New("ssh launch receipt is incomplete")
	// ErrReceiptMismatch means the receipt is complete but names a different
	// owner than the caller expected.
	ErrReceiptMismatch = errors.New("ssh launch receipt does not match the expected owner")
)

// TargetBinding is the saved SSH connection tuple a receipt is bound to.
// It uses the resolved target values (never a host alias) so the binding
// survives a config edit that only changes how the host is named.
type TargetBinding struct {
	Host            string `json:"host"`
	Port            string `json:"port"`
	User            string `json:"user"`
	HostFingerprint string `json:"host_fingerprint"`
}

func (t TargetBinding) Validate() error {
	if strings.TrimSpace(t.Host) == "" || strings.TrimSpace(t.Port) == "" ||
		strings.TrimSpace(t.User) == "" || strings.TrimSpace(t.HostFingerprint) == "" {
		return fmt.Errorf("%w: target binding", ErrIncompleteReceipt)
	}
	return nil
}

func (t TargetBinding) equal(other TargetBinding) bool {
	return t == other
}

// Owner is the exact task, session, controller, and path set a receipt
// belongs to. ControllerInstanceID is the agentctl instance ID the backend
// recorded for this launch; it changes when a replacement controller is
// created.
type Owner struct {
	TaskID               string        `json:"task_id"`
	SessionID            string        `json:"session_id"`
	ControllerInstanceID string        `json:"controller_instance_id"`
	TaskDir              string        `json:"task_dir"`
	SessionDir           string        `json:"session_dir"`
	Target               TargetBinding `json:"target"`
}

func (o Owner) Validate() error {
	if strings.TrimSpace(o.TaskID) == "" || strings.TrimSpace(o.SessionID) == "" ||
		strings.TrimSpace(o.ControllerInstanceID) == "" || strings.TrimSpace(o.TaskDir) == "" ||
		strings.TrimSpace(o.SessionDir) == "" {
		return fmt.Errorf("%w: owner binding", ErrIncompleteReceipt)
	}
	return o.Target.Validate()
}

func (o Owner) equal(other Owner) bool {
	return o.TaskID == other.TaskID &&
		o.SessionID == other.SessionID &&
		o.ControllerInstanceID == other.ControllerInstanceID &&
		o.TaskDir == other.TaskDir &&
		o.SessionDir == other.SessionDir &&
		o.Target.equal(other.Target)
}

// Binding is the backend-issued launch request handed to the managed agentctl
// child through AgentctlStartupConfig. The child echoes it back inside the
// receipt it self-captures.
type Binding struct {
	Generation string `json:"generation"`
	Owner      Owner  `json:"owner"`
}

func (b Binding) Validate() error {
	if !isGeneration(b.Generation) {
		return fmt.Errorf("%w: launch generation", ErrIncompleteReceipt)
	}
	return b.Owner.Validate()
}

// Receipt is the immutable, self-captured proof returned by the controller
// during the nonce-authenticated handshake.
type Receipt struct {
	Version    int                      `json:"version"`
	Generation string                   `json:"generation"`
	Owner      Owner                    `json:"owner"`
	UID        int                      `json:"uid"`
	Process    processidentity.Identity `json:"process"`
}

// Validate checks the receipt is complete and self-consistent. It does not
// compare it against an expected owner; use ValidateFor for that.
func (r Receipt) Validate() error {
	if r.Version != Version {
		return fmt.Errorf("%w: unsupported receipt version %d", ErrIncompleteReceipt, r.Version)
	}
	if !isGeneration(r.Generation) {
		return fmt.Errorf("%w: launch generation", ErrIncompleteReceipt)
	}
	if err := r.Owner.Validate(); err != nil {
		return err
	}
	if err := r.Process.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrIncompleteReceipt, err)
	}
	if r.UID < 0 {
		return fmt.Errorf("%w: effective uid", ErrIncompleteReceipt)
	}
	return nil
}

// ValidateFor requires this receipt to describe exactly the expected owner.
// A complete receipt for a different launch is a mismatch, not evidence.
func (r Receipt) ValidateFor(expected Owner) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if !r.Owner.equal(expected) {
		return ErrReceiptMismatch
	}
	return nil
}

// ValidateBinding requires the receipt to match a backend-issued binding,
// including the per-attempt generation.
func (r Receipt) ValidateBinding(binding Binding) error {
	if err := binding.Validate(); err != nil {
		return err
	}
	if err := r.ValidateFor(binding.Owner); err != nil {
		return err
	}
	if r.Generation != binding.Generation {
		return ErrReceiptMismatch
	}
	return nil
}

// Metadata projects the receipt onto the executors_running.metadata shape.
// The JSON round trip keeps the birth token a string, so Linux start ticks
// never pass through a float.
func (r Receipt) Metadata() (map[string]interface{}, error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("marshal ssh launch receipt: %w", err)
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil, fmt.Errorf("project ssh launch receipt: %w", err)
	}
	return metadata, nil
}

// ReceiptFromMetadata reads a receipt out of executors_running.metadata.
// A missing or malformed value returns ok=false: an unreadable receipt is
// absent proof, never a weaker one.
func ReceiptFromMetadata(metadata map[string]interface{}) (Receipt, bool) {
	if metadata == nil {
		return Receipt{}, false
	}
	raw, ok := metadata[MetadataKey]
	if !ok || raw == nil {
		return Receipt{}, false
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return Receipt{}, false
	}
	var receipt Receipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return Receipt{}, false
	}
	if err := receipt.Validate(); err != nil {
		return Receipt{}, false
	}
	return receipt, true
}

// NewGeneration returns a fresh random 128-bit launch generation. Every launch
// attempt must use its own value; reusing one would let a retried attempt
// inherit the previous attempt's proof.
func NewGeneration() (string, error) {
	buf := make([]byte, generationBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate ssh launch generation: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func isGeneration(value string) bool {
	if len(value) != hex.EncodedLen(generationBytes) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
