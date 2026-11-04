// Package sandboxruntime coordinates provider-neutral, core-owned sandbox attempts.
package sandboxruntime

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"sync"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/common/sandboxmcp"
	"github.com/kandev/kandev/internal/common/sandboxproxy"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kdlbs/kandev/pluginsdk"
)

type PrepareRequest struct {
	MCPPaths                        []string
	RestartOf                       string
	ProtectedPorts                  []uint16
	Selection                       policy.SelectionContext
	SessionID, ExecutionID, WorkDir string
	Generation                      uint64
	Arguments                       []string
	EnvironmentOverrides            map[string]string
	MCPForward                      func(string, uint64, string) sandboxmcp.Forward
}

type Manager struct {
	audit     auditJobState
	policies  *policy.Service
	providers *plugins.Service
	base      string
	channels  *policy.ApprovalChannels
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	attempts  map[string]*Prepared
	closed    bool
}

type Prepared struct {
	selectionDigest string
	manager         *Manager
	orgID           string
	binding         plugins.SandboxProviderBinding
	description     *pluginsdk.SandboxProviderDescription
	policy          *pluginsdk.SandboxPolicySnapshot
	request         *pluginsdk.SandboxRequestContext
	launch          *contract.Launch
	agentState      *contract.AgentState
	files           *contract.RuntimeFiles
	channel         *policy.ApprovalChannel
	egress          *sandboxproxy.Proxy
	mcp             *sandboxmcp.Relay
	mcpPaths        []string
	runtimeNetwork  []contract.NetworkGrant
	protectedPorts  []uint16
	constraints     contract.OverrideConstraints
	mu              sync.Mutex
	owner           *contract.ProcessOwner
	startDispatched bool
	closed          bool
}

func NewManager(policies *policy.Service, providers *plugins.Service, base string) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{audit: auditJobState{epoch: uuid.NewString(), jobs: map[string]*auditJob{}}, policies: policies, providers: providers, base: base, ctx: ctx, cancel: cancel, attempts: map[string]*Prepared{}}
	m.channels = policy.NewApprovalChannels(policies, m.normalizeApproval, m.VerifyLive)
	return m
}

func (p *Prepared) Launch() *contract.Launch { return p.launch.Clone() }

func (m *Manager) Close() error {
	m.audit.admission.Lock()
	defer m.audit.admission.Unlock()
	m.mu.Lock()
	m.closed = true
	pending := make([]*Prepared, 0, len(m.attempts))
	for _, attempt := range m.attempts {
		pending = append(pending, attempt)
	}
	m.mu.Unlock()
	m.cancel()
	m.audit.wait.Wait()
	err := m.channels.Close()
	m.mu.Lock()
	if m.audit.persistenceFailed {
		err = errors.Join(err, policy.ErrConflict)
	}
	m.mu.Unlock()
	for _, attempt := range pending {
		err = errors.Join(err, attempt.Close())
	}
	return err
}

func (m *Manager) retained(orgID, attemptID string) (*Prepared, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	attempt := m.attempts[attemptID]
	if m.closed || attempt == nil || attempt.orgID != orgID {
		return nil, policy.ErrConflict
	}
	return attempt, nil
}
