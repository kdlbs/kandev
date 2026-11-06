package lifecycle

import (
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

// projectMCPDeliveryState keeps execution-bound project delivery evidence
// private to one execution until the common configure handoff.
type projectMCPDeliveryState struct {
	mu       sync.Mutex
	delivery *agentctltypes.ProjectMCPDelivery
	remote   *projectMCPRemoteState
}

// projectMCPRemoteState binds a preparation generation to the helper client
// that accepted it. Cleanup must target that helper even after replacement.
type projectMCPRemoteState struct {
	client     *agentctl.Client
	generation uint64
}

func (e *AgentExecution) setProjectMCPDelivery(delivery *agentctltypes.ProjectMCPDelivery) {
	if e == nil {
		return
	}
	e.projectMCP.mu.Lock()
	e.projectMCP.delivery = delivery.Clone()
	e.projectMCP.mu.Unlock()
}

func (e *AgentExecution) clearProjectMCPDelivery() {
	e.setProjectMCPDelivery(nil)
}

func (e *AgentExecution) projectMCPDeliverySnapshot() *agentctltypes.ProjectMCPDelivery {
	if e == nil {
		return nil
	}
	e.projectMCP.mu.Lock()
	defer e.projectMCP.mu.Unlock()
	return e.projectMCP.delivery.Clone()
}

func (e *AgentExecution) setProjectMCPRemotePrepared(client *agentctl.Client, generation uint64) {
	if e == nil {
		return
	}
	e.projectMCP.mu.Lock()
	e.projectMCP.remote = &projectMCPRemoteState{client: client, generation: generation}
	e.projectMCP.mu.Unlock()
}

func (e *AgentExecution) projectMCPRemotePrepared() (client *agentctl.Client, generation uint64) {
	if e == nil {
		return nil, 0
	}
	e.projectMCP.mu.Lock()
	defer e.projectMCP.mu.Unlock()
	if e.projectMCP.remote == nil {
		return nil, 0
	}
	return e.projectMCP.remote.client, e.projectMCP.remote.generation
}

func (e *AgentExecution) clearProjectMCPRemotePrepared() {
	if e == nil {
		return
	}
	e.projectMCP.mu.Lock()
	e.projectMCP.remote = nil
	e.projectMCP.mu.Unlock()
}

// mcpProjectDeliveryFor returns the execution-bound evidence to configure for
// this helper. Evidence prepared for a replaced helper or an invalid record
// fails closed instead of authorizing a successor process.
func (e *AgentExecution) mcpProjectDeliveryFor(client *agentctl.Client) (*agentctltypes.ProjectMCPDelivery, error) {
	delivery := e.projectMCPDeliverySnapshot()
	if delivery == nil {
		return nil, nil
	}
	if err := delivery.Validate(); err != nil {
		return nil, fmt.Errorf("invalid project MCP delivery evidence: %w", err)
	}
	if client == nil || delivery.InstanceID != client.InstanceID() {
		return nil, errors.New("project MCP delivery evidence belongs to a replaced agent helper")
	}
	return delivery, nil
}

// projectMCPDeliveryEvidence builds the record after a successful project-file
// write. Missing strategy support, a bound package manifest, a helper identity,
// or any injected package yields no evidence, so admission keeps rejecting.
func projectMCPDeliveryEvidence(
	execution *AgentExecution,
	strategy mcpconfig.PassthroughMCPStrategy,
	servers []agentctltypes.McpServer,
	client *agentctl.Client,
	workspace string,
) *agentctltypes.ProjectMCPDelivery {
	strategyKey, supported := mcpconfig.ProjectDeliveryStrategyKey(strategy)
	if !supported || client == nil {
		return nil
	}
	instanceID := client.InstanceID()
	if instanceID == "" {
		return nil
	}
	value, bound := execution.metadataValue(p.BuiltinBindingMetadataKey)
	if !bound {
		return nil
	}
	profile, err := p.ParseBuiltinBindingMetadata(value)
	if err != nil {
		return nil
	}
	digest, err := p.BuiltinBindingDigest(*profile)
	if err != nil {
		return nil
	}
	workspace = agentctltypes.CanonicalProjectMCPDeliveryWorkspace(workspace)
	if workspace == "" {
		return nil
	}
	packages := deliveredBuiltinPackages(servers)
	if len(packages) == 0 {
		return nil
	}
	return &agentctltypes.ProjectMCPDelivery{
		Version:       agentctltypes.ProjectMCPDeliveryVersion,
		Strategy:      strategyKey,
		Workspace:     workspace,
		TaskID:        execution.TaskID,
		InstanceID:    instanceID,
		BindingDigest: digest,
		Packages:      packages,
	}
}

// deliveredBuiltinPackages returns the exact manifest packages present in the
// generated operation. HTTP/SSE alternatives share one package identity.
func deliveredBuiltinPackages(servers []agentctltypes.McpServer) []string {
	packages := make([]string, 0, len(servers))
	for _, server := range servers {
		if server.Name == "" || !server.IsInjectedBuiltin() {
			continue
		}
		pkg := string(server.BuiltinPackage)
		if !slices.Contains(packages, pkg) {
			packages = append(packages, pkg)
		}
	}
	slices.Sort(packages)
	return packages
}
