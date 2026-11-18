package plugins

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	filePreviewAsk   = "ask"
	filePreviewAllow = "allow"
	filePreviewDeny  = "deny"
)

var errFilePreviewPolicy = errors.New("file preview policy unavailable")

type filePreviewScope struct{ UserID, WorkspaceID, InstallationID, Digest string }
type filePreviewPolicy struct {
	Mode     string `json:"mode"`
	Revision uint64 `json:"revision"`
}
type filePreviewPolicyEvent struct {
	Mode     string    `json:"mode"`
	Revision uint64    `json:"revision"`
	At       time.Time `json:"at"`
}
type filePreviewPolicyRow struct {
	Events []filePreviewPolicyEvent `json:"events,omitempty"`
	filePreviewPolicy
	Digest string `json:"digest"`
}
type filePreviewPolicies struct {
	dir string
	mu  sync.Mutex
}

func newFilePreviewPolicies(dir string) *filePreviewPolicies { return &filePreviewPolicies{dir: dir} }

func (s filePreviewScope) key() (string, error) {
	for _, value := range []string{s.UserID, s.WorkspaceID, s.InstallationID, s.Digest} {
		if value == "" || len(value) > 256 || strings.ContainsRune(value, 0) {
			return "", errFilePreviewPolicy
		}
	}
	return s.UserID + "\x00" + s.WorkspaceID + "\x00" + s.InstallationID, nil
}

func (p *filePreviewPolicies) load() (map[string]filePreviewPolicyRow, error) {
	f, err := os.Open(filepath.Join(p.dir, "file-preview-policies.json"))
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]filePreviewPolicyRow), nil
	}
	if err != nil {
		return nil, errFilePreviewPolicy
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil, errFilePreviewPolicy
	}
	var rows map[string]filePreviewPolicyRow
	if json.Unmarshal(data, &rows) != nil || rows == nil {
		return nil, errFilePreviewPolicy
	}
	for _, row := range rows {
		if !validFilePreviewMode(row.Mode) || row.Revision == 0 || row.Digest == "" || len(row.Events) > 32 {
			return nil, errFilePreviewPolicy
		}
	}
	return rows, nil
}

func validFilePreviewMode(mode string) bool {
	return mode == filePreviewAsk || mode == filePreviewAllow || mode == filePreviewDeny
}

func (p *filePreviewPolicies) get(scope filePreviewScope) (filePreviewPolicy, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key, err := scope.key()
	if err != nil {
		return filePreviewPolicy{}, err
	}
	rows, err := p.load()
	if err != nil {
		return filePreviewPolicy{}, err
	}
	return p.policyForDigest(rows, key, scope.Digest)
}

// policyForDigest runs under p.mu for both reads and writes. A package change
// invalidates the prior revision before a caller can apply a policy decision.
func (p *filePreviewPolicies) policyForDigest(rows map[string]filePreviewPolicyRow, key, digest string) (filePreviewPolicy, error) {
	row := rows[key]
	if row.Digest == digest {
		return row.filePreviewPolicy, nil
	}
	policy := filePreviewPolicy{Mode: filePreviewAsk, Revision: row.Revision}
	if row.Revision == 0 {
		return policy, nil
	}
	if row.Revision == ^uint64(0) {
		return filePreviewPolicy{}, errFilePreviewPolicy
	}
	policy.Revision++
	rows[key] = filePreviewPolicyRow{filePreviewPolicy: policy, Digest: digest, Events: appendFilePreviewPolicyEvent(row.Events, policy)}
	if err := p.save(rows); err != nil {
		return filePreviewPolicy{}, errFilePreviewPolicy
	}
	return policy, nil
}

func (p *filePreviewPolicies) set(scope filePreviewScope, mode string, revision uint64) (filePreviewPolicy, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key, err := scope.key()
	if err != nil || !validFilePreviewMode(mode) {
		return filePreviewPolicy{}, errFilePreviewPolicy
	}
	rows, err := p.load()
	if err != nil {
		return filePreviewPolicy{}, err
	}
	current, err := p.policyForDigest(rows, key, scope.Digest)
	if err != nil {
		return filePreviewPolicy{}, err
	}
	if current.Revision != revision || revision == ^uint64(0) {
		return filePreviewPolicy{}, ErrApprovalRevisionConflict
	}
	policy := filePreviewPolicy{Mode: mode, Revision: revision + 1}
	rows[key] = filePreviewPolicyRow{filePreviewPolicy: policy, Digest: scope.Digest, Events: appendFilePreviewPolicyEvent(rows[key].Events, policy)}
	if err := p.save(rows); err != nil {
		return filePreviewPolicy{}, errFilePreviewPolicy
	}
	return policy, nil
}

func (p *filePreviewPolicies) save(rows map[string]filePreviewPolicyRow) error {
	data, err := json.Marshal(rows)
	if err != nil || len(data) > 1<<20 {
		return errFilePreviewPolicy
	}
	if err := os.MkdirAll(p.dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(p.dir, ".file-preview-policies-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(f.Name(), filepath.Join(p.dir, "file-preview-policies.json")); err != nil {
		return err
	}
	return syncApprovalLedgerDirectory(p.dir)
}

func appendFilePreviewPolicyEvent(events []filePreviewPolicyEvent, policy filePreviewPolicy) []filePreviewPolicyEvent {
	if len(events) >= 32 {
		events = events[len(events)-31:]
	}
	return append(events, filePreviewPolicyEvent{Mode: policy.Mode, Revision: policy.Revision, At: time.Now().UTC()})
}
