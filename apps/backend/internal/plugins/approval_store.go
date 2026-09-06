package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type ApprovalState string

const (
	ApprovalStateActive  ApprovalState = "active"
	ApprovalStateRevoked ApprovalState = "revoked"
)

// HumanPolicyVersionImmutable is the only Human-policy version H6 currently
// admits. H0 defines the Human-reserved deny intersection (merge, deploy,
// release, history rewrite, cross-workspace access, secret-scope expansion)
// as an immutable, singular policy with no versioned variants yet. The field
// is stored on every approval row so a future policy revision has a stable
// place to record which policy an approval was granted under; it is not a
// caller-configurable input today.
const HumanPolicyVersionImmutable = "immutable"

type CapabilityApproval struct {
	InstallationID     string        `json:"installation_id"`
	WorkspaceID        string        `json:"workspace_id"`
	Revision           uint64        `json:"revision"`
	ManifestDigest     string        `json:"manifest_digest"`
	CapabilityIDs      []string      `json:"capability_ids"`
	State              ApprovalState `json:"state"`
	HumanActor         string        `json:"human_actor"`
	HumanPolicyVersion string        `json:"human_policy_version"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
	TombstonedAt       *time.Time    `json:"tombstoned_at,omitempty"`
}

type CapabilityApprovalEventType string

const (
	CapabilityApprovalEventGrant         CapabilityApprovalEventType = "grant"
	CapabilityApprovalEventNarrow        CapabilityApprovalEventType = "narrow"
	CapabilityApprovalEventRevoke        CapabilityApprovalEventType = "revoke"
	CapabilityApprovalEventUpgradeReview CapabilityApprovalEventType = "upgrade_review"
)

type CapabilityApprovalEvent struct {
	AuditID        string                      `json:"audit_id"`
	InstallationID string                      `json:"installation_id"`
	WorkspaceID    string                      `json:"workspace_id"`
	BeforeRevision uint64                      `json:"before_revision"`
	AfterRevision  uint64                      `json:"after_revision"`
	BeforeDigest   string                      `json:"before_digest"`
	AfterDigest    string                      `json:"after_digest"`
	Actor          string                      `json:"actor"`
	Reason         string                      `json:"reason"`
	Type           CapabilityApprovalEventType `json:"type"`
	ObservedAt     time.Time                   `json:"observed_at"`
}

type approvalLedgerFile struct {
	Approvals         map[string]CapabilityApproval       `json:"approvals"`
	Events            []CapabilityApprovalEvent           `json:"events"`
	Tombstones        map[string]time.Time                `json:"tombstones"`
	Idempotency       map[string]CapabilityApproval       `json:"idempotency,omitempty"`
	IdempotencyInputs map[string]approvalIdempotencyInput `json:"idempotency_inputs,omitempty"`
}

type approvalIdempotencyInput struct {
	Revision       uint64   `json:"revision"`
	ManifestDigest string   `json:"manifest_digest,omitempty"`
	CapabilityIDs  []string `json:"capability_ids,omitempty"`
	Actor          string   `json:"actor"`
	Reason         string   `json:"reason"`
}

var (
	ErrApprovalRevisionConflict       = errors.New("plugins: approval revision conflict")
	ErrApprovalIdempotencyConflict    = errors.New("plugins: approval idempotency conflict")
	ErrApprovalInstallationTombstoned = errors.New("plugins: installation approval tombstoned")
)

type approvalLedger struct {
	dir string
	mu  sync.Mutex
}

func newApprovalLedger(dir string) *approvalLedger {
	return &approvalLedger{dir: dir}
}

func (l *approvalLedger) path() string {
	return filepath.Join(l.dir, "approvals.json")
}

func (l *approvalLedger) load() (*approvalLedgerFile, error) {
	data, err := os.ReadFile(l.path())
	if err != nil {
		if os.IsNotExist(err) {
			return &approvalLedgerFile{
				Approvals:         map[string]CapabilityApproval{},
				Tombstones:        map[string]time.Time{},
				Idempotency:       map[string]CapabilityApproval{},
				IdempotencyInputs: map[string]approvalIdempotencyInput{},
			}, nil
		}
		return nil, err
	}
	var file approvalLedgerFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	if file.Approvals == nil {
		file.Approvals = map[string]CapabilityApproval{}
	}
	if file.Tombstones == nil {
		file.Tombstones = map[string]time.Time{}
	}
	if file.Idempotency == nil {
		file.Idempotency = map[string]CapabilityApproval{}
	}
	if file.IdempotencyInputs == nil {
		file.IdempotencyInputs = map[string]approvalIdempotencyInput{}
	}
	return &file, nil
}

func (l *approvalLedger) save(file *approvalLedgerFile) error {
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(l.dir, ".approvals-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, l.path())
}

func approvalKey(installationID, workspaceID string) string {
	return installationID + "\x00" + workspaceID
}

func approvalIdempotencyKey(eventType CapabilityApprovalEventType, auditID, installationID, workspaceID string) string {
	return string(eventType) + "\x00" + auditID + "\x00" + installationID + "\x00" + workspaceID
}

func (l *approvalLedger) grant(installationID, workspaceID string, revision uint64, manifestDigest string, capabilityIDs []string, actor, reason, auditID string, at time.Time) (CapabilityApproval, error) {
	if revision == 0 {
		return CapabilityApproval{}, ErrApprovalRevisionConflict
	}
	if strings.TrimSpace(auditID) == "" {
		return CapabilityApproval{}, errors.New("plugins: approval audit id is required")
	}
	canonical, err := CanonicalCapabilityList(capabilityIDs)
	if err != nil {
		return CapabilityApproval{}, err
	}
	capabilityIDs = canonical
	l.mu.Lock()
	defer l.mu.Unlock()
	file, err := l.load()
	if err != nil {
		return CapabilityApproval{}, err
	}
	key := approvalKey(installationID, workspaceID)
	current := file.Approvals[key]
	if _, tombstoned := file.Tombstones[installationID]; tombstoned {
		return CapabilityApproval{}, ErrApprovalInstallationTombstoned
	}
	if replayed, found, err := approvalGrantReplay(file, installationID, workspaceID, revision, manifestDigest, capabilityIDs, actor, reason, auditID); found || err != nil {
		return replayed, err
	}
	if !approvalRevisionFollows(current.Revision, revision) {
		return CapabilityApproval{}, ErrApprovalRevisionConflict
	}
	approval := CapabilityApproval{
		InstallationID:     installationID,
		WorkspaceID:        workspaceID,
		Revision:           revision,
		ManifestDigest:     manifestDigest,
		CapabilityIDs:      append([]string{}, capabilityIDs...),
		State:              ApprovalStateActive,
		HumanActor:         actor,
		HumanPolicyVersion: HumanPolicyVersionImmutable,
		CreatedAt:          at,
		UpdatedAt:          at,
	}
	if current.Revision != 0 {
		approval.CreatedAt = current.CreatedAt
	}
	file.Approvals[key] = approval
	file.Idempotency[approvalIdempotencyKey(CapabilityApprovalEventGrant, auditID, installationID, workspaceID)] = approval
	file.IdempotencyInputs[approvalIdempotencyKey(CapabilityApprovalEventGrant, auditID, installationID, workspaceID)] = approvalIdempotencyInput{
		Revision: revision, ManifestDigest: manifestDigest, CapabilityIDs: append([]string{}, capabilityIDs...), Actor: actor, Reason: reason,
	}
	file.Events = append(file.Events, CapabilityApprovalEvent{
		AuditID: auditID, InstallationID: installationID, WorkspaceID: workspaceID,
		BeforeRevision: current.Revision, AfterRevision: revision,
		BeforeDigest: current.ManifestDigest, AfterDigest: manifestDigest,
		Actor: actor, Reason: reason, Type: CapabilityApprovalEventGrant, ObservedAt: at,
	})
	return approval, l.save(file)
}

func approvalGrantReplay(file *approvalLedgerFile, installationID, workspaceID string, revision uint64, manifestDigest string, capabilityIDs []string, actor, reason, auditID string) (CapabilityApproval, bool, error) {
	key := approvalIdempotencyKey(CapabilityApprovalEventGrant, auditID, installationID, workspaceID)
	replayed, ok := file.Idempotency[key]
	if !ok {
		return CapabilityApproval{}, false, nil
	}
	input, inputOK := file.IdempotencyInputs[key]
	if inputOK && input.Revision == revision && input.ManifestDigest == manifestDigest && equalStrings(input.CapabilityIDs, capabilityIDs) && input.Actor == actor && input.Reason == reason {
		return replayed, true, nil
	}
	return CapabilityApproval{}, true, ErrApprovalIdempotencyConflict
}

func approvalRevisionFollows(current, next uint64) bool {
	if current == 0 {
		return next == 1
	}
	return next == current+1
}

func (l *approvalLedger) revoke(installationID, workspaceID string, actor, reason, auditID string, at time.Time) (CapabilityApproval, error) {
	return l.revokeIfRevision(installationID, workspaceID, 0, actor, reason, auditID, at, true)
}

func (l *approvalLedger) revokeIfRevision(installationID, workspaceID string, expectedRevision uint64, actor, reason, auditID string, at time.Time, allowCurrent bool) (CapabilityApproval, error) {
	if strings.TrimSpace(auditID) == "" {
		return CapabilityApproval{}, errors.New("plugins: approval audit id is required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	file, err := l.load()
	if err != nil {
		return CapabilityApproval{}, err
	}
	key := approvalKey(installationID, workspaceID)
	current, ok := file.Approvals[key]
	if !ok {
		return CapabilityApproval{}, fmt.Errorf("plugins: approval not found")
	}
	effectiveExpectedRevision := expectedRevision
	if allowCurrent {
		effectiveExpectedRevision = current.Revision
	}
	if auditID != "" {
		idempotencyKey := approvalIdempotencyKey(CapabilityApprovalEventRevoke, auditID, installationID, workspaceID)
		if replayed, ok := file.Idempotency[idempotencyKey]; ok {
			input, inputOK := file.IdempotencyInputs[idempotencyKey]
			// Current-revision callers intentionally pass allowCurrent=true and
			// do not retain the pre-mutation revision. Once the first revoke
			// advances the row, the audit identity is the only stable lookup for
			// an exact retry. Explicit-revision callers remain bound to the
			// persisted pre-revoke revision.
			if inputOK && (allowCurrent || input.Revision == expectedRevision) && input.Actor == actor && input.Reason == reason {
				return replayed, nil
			}
			return CapabilityApproval{}, ErrApprovalIdempotencyConflict
		}
	}
	expectedRevision = effectiveExpectedRevision
	if current.Revision != expectedRevision {
		return CapabilityApproval{}, ErrApprovalRevisionConflict
	}
	next := current
	next.Revision++
	next.State = ApprovalStateRevoked
	next.HumanActor = actor
	next.UpdatedAt = at
	file.Approvals[key] = next
	if auditID != "" {
		idempotencyKey := approvalIdempotencyKey(CapabilityApprovalEventRevoke, auditID, installationID, workspaceID)
		file.Idempotency[idempotencyKey] = next
		file.IdempotencyInputs[idempotencyKey] = approvalIdempotencyInput{Revision: expectedRevision, Actor: actor, Reason: reason}
	}
	file.Events = append(file.Events, CapabilityApprovalEvent{
		AuditID: auditID, InstallationID: installationID, WorkspaceID: workspaceID,
		BeforeRevision: current.Revision, AfterRevision: next.Revision,
		BeforeDigest: current.ManifestDigest, AfterDigest: current.ManifestDigest,
		Actor: actor, Reason: reason, Type: CapabilityApprovalEventRevoke, ObservedAt: at,
	})
	return next, l.save(file)
}

func (l *approvalLedger) tombstoneInstallation(installationID string, at time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	file, err := l.load()
	if err != nil {
		return err
	}
	if _, ok := file.Tombstones[installationID]; !ok {
		file.Tombstones[installationID] = at
	}
	for key, approval := range file.Approvals {
		if approval.InstallationID != installationID {
			continue
		}
		if approval.TombstonedAt != nil {
			continue
		}
		approval.Revision++
		approval.State = ApprovalStateRevoked
		approval.UpdatedAt = at
		tombstonedAt := at
		approval.TombstonedAt = &tombstonedAt
		file.Approvals[key] = approval
		file.Events = append(file.Events, CapabilityApprovalEvent{
			AuditID:        CanonicalApprovalDigest(installationID, approval.WorkspaceID, fmt.Sprint(approval.Revision), "uninstall"),
			InstallationID: installationID, WorkspaceID: approval.WorkspaceID,
			BeforeRevision: approval.Revision - 1, AfterRevision: approval.Revision,
			BeforeDigest: approval.ManifestDigest, AfterDigest: approval.ManifestDigest,
			Actor: "host", Reason: "installation uninstalled", Type: CapabilityApprovalEventRevoke, ObservedAt: at,
		})
	}
	return l.save(file)
}

func (l *approvalLedger) reviewManifestChange(installationID, manifestDigest string, manifestCapabilities []string, at time.Time) error {
	canonical, err := CanonicalCapabilityList(manifestCapabilities)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	file, err := l.load()
	if err != nil {
		return err
	}
	for key, approval := range file.Approvals {
		if approval.InstallationID != installationID || approval.ManifestDigest == manifestDigest || approval.TombstonedAt != nil {
			continue
		}
		current := approval
		approval.Revision++
		approval.ManifestDigest = manifestDigest
		approval.CapabilityIDs = intersectStrings(approval.CapabilityIDs, canonical)
		approval.UpdatedAt = at
		file.Approvals[key] = approval
		file.Events = append(file.Events, CapabilityApprovalEvent{
			AuditID:        CanonicalApprovalDigest(installationID, approval.WorkspaceID, fmt.Sprint(approval.Revision), manifestDigest, "manifest-review"),
			InstallationID: installationID, WorkspaceID: approval.WorkspaceID,
			BeforeRevision: current.Revision, AfterRevision: approval.Revision,
			BeforeDigest: current.ManifestDigest, AfterDigest: manifestDigest,
			Actor: "host", Reason: "installation manifest changed", Type: CapabilityApprovalEventUpgradeReview, ObservedAt: at,
		})
	}
	return l.save(file)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func intersectStrings(a, b []string) []string {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(b))
	for _, v := range b {
		allowed[v] = struct{}{}
	}
	out := make([]string, 0, len(a))
	for _, v := range a {
		if _, ok := allowed[v]; ok {
			out = append(out, v)
		}
	}
	return out
}

func (l *approvalLedger) get(installationID, workspaceID string) (CapabilityApproval, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	file, err := l.load()
	if err != nil {
		return CapabilityApproval{}, false, err
	}
	approval, ok := file.Approvals[approvalKey(installationID, workspaceID)]
	return approval, ok, nil
}

func (l *approvalLedger) listByInstallation(installationID string) ([]CapabilityApproval, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	file, err := l.load()
	if err != nil {
		return nil, err
	}
	var out []CapabilityApproval
	for _, approval := range file.Approvals {
		if approval.InstallationID == installationID {
			out = append(out, approval)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].WorkspaceID == out[j].WorkspaceID {
			return out[i].Revision < out[j].Revision
		}
		return out[i].WorkspaceID < out[j].WorkspaceID
	})
	return out, nil
}
