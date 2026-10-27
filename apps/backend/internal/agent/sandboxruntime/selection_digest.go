package sandboxruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/plugins"
)

// SelectionDigest binds discovery results to policy bindings and the live installation.
func (m *Manager) SelectionDigest(ctx context.Context, resolved *policy.Resolved) (string, error) {
	if resolved == nil || resolved.Revision == nil {
		return "", policy.ErrIneligible
	}
	binding, err := m.selectProvider(ctx, resolved.Revision.Policy.InstallationID, resolved.Revision.Policy.ProviderKey)
	if err != nil {
		return "", err
	}
	return selectionDigest(resolved, binding)
}

func selectionDigest(resolved *policy.Resolved, binding plugins.SandboxProviderBinding) (string, error) {
	encoded, err := json.Marshal(struct {
		Policy   *policy.Resolved               `json:"policy"`
		Provider plugins.SandboxProviderBinding `json:"provider"`
	}{resolved, binding})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func (p *Prepared) SelectionDigest() string { return p.selectionDigest }
