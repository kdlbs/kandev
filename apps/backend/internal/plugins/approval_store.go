package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	Approvals  map[string]CapabilityApproval `json:"approvals"`
	Events     []CapabilityApprovalEvent     `json:"events"`
	Tombstones map[string]time.Time          `json:"tombstones"`
}

var (
	ErrApprovalRevisionConflict    = errors.New("plugins: approval revision conflict")
	ErrApprovalIdempotencyConflict = errors.New("plugins: approval idempotency conflict")
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
				Approvals:  map[string]CapabilityApproval{},
				Tombstones: map[string]time.Time{},
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

func (l *approvalLedger) grant(installationID, workspaceID string, revision uint64, manifestDigest string, capabilityIDs []string, actor, reason, auditID string, at time.Time) (CapabilityApproval, error) {
	if revision == 0 {
		return CapabilityApproval{}, ErrApprovalRevisionConflict
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
	if auditID != "" {
		for _, event := range file.Events {
			if event.AuditID != auditID || event.InstallationID != installationID || event.WorkspaceID != workspaceID {
				continue
			}
			if current.Revision == revision && current.ManifestDigest == manifestDigest && equalStrings(current.CapabilityIDs, capabilityIDs) {
				return current, nil
			}
			return CapabilityApproval{}, ErrApprovalIdempotencyConflict
		}
	}
	if current.Revision != 0 && current.Revision != revision-1 {
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
	file.Events = append(file.Events, CapabilityApprovalEvent{
		AuditID: auditID, InstallationID: installationID, WorkspaceID: workspaceID,
		BeforeRevision: current.Revision, AfterRevision: revision,
		BeforeDigest: current.ManifestDigest, AfterDigest: manifestDigest,
		Actor: actor, Reason: reason, Type: CapabilityApprovalEventGrant, ObservedAt: at,
	})
	return approval, l.save(file)
}

func (l *approvalLedger) revoke(installationID, workspaceID string, actor, reason, auditID string, at time.Time) (CapabilityApproval, error) {
	return l.revokeIfRevision(installationID, workspaceID, 0, actor, reason, auditID, at, true)
}

func (l *approvalLedger) revokeIfRevision(installationID, workspaceID string, expectedRevision uint64, actor, reason, auditID string, at time.Time, allowCurrent bool) (CapabilityApproval, error) {
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
	for _, event := range file.Events {
		if event.AuditID == auditID && event.InstallationID == installationID && event.WorkspaceID == workspaceID && event.Type == CapabilityApprovalEventRevoke {
			return current, nil
		}
	}
	if allowCurrent {
		expectedRevision = current.Revision
	}
	if current.Revision != expectedRevision {
		return CapabilityApproval{}, ErrApprovalRevisionConflict
	}
	next := current
	next.Revision++
	next.State = ApprovalStateRevoked
	next.HumanActor = actor
	next.UpdatedAt = at
	file.Approvals[key] = next
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
	file.Tombstones[installationID] = at
	for key, approval := range file.Approvals {
		if approval.InstallationID != installationID {
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
