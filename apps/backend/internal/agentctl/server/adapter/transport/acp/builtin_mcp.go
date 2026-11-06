package acp

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

// projectMCPDeliveryReasonCode names the alternate delivery channel in
// attachment evidence. It never implies provider consumption or health.
const projectMCPDeliveryReasonCode = "project_config_materialized"

// requireBuiltinMCPDelivery accepts each requested host-injected package when
// the ACP wire list delivers it or the execution-bound project record names it.
// This is a universal condition over selected packages: Core delivery never
// authorizes a missing optional package.
func requireBuiltinMCPDelivery(requested, selected []types.McpServer, delivery *types.ProjectMCPDelivery) error {
	delivered := make(map[p.PackageID]bool, len(selected))
	for _, server := range selected {
		if server.IsInjectedBuiltin() && server.BuiltinPackage != "" {
			delivered[server.BuiltinPackage] = true
		}
	}
	projectDelivered := validProjectMCPDelivery(delivery)
	for _, server := range requested {
		if !server.IsInjectedBuiltin() {
			continue
		}
		if delivered[server.BuiltinPackage] || projectDelivered.Delivers(server.BuiltinPackage) {
			continue
		}
		return fmt.Errorf("selected built-in MCP server %q is unsupported by this agent; use an agent with HTTP or SSE MCP support", server.Name)
	}
	return nil
}

// validProjectMCPDelivery returns the record only when its structural contract
// holds. Process configuration already validated the identity binding; this
// keeps directly constructed adapters fail-closed too.
func validProjectMCPDelivery(delivery *types.ProjectMCPDelivery) *types.ProjectMCPDelivery {
	if delivery == nil || delivery.Validate() != nil {
		return nil
	}
	return delivery
}

// projectDeliveryEvidence returns the validated record for this adapter, or
// nil when the process was configured without project delivery.
func (a *Adapter) projectDeliveryEvidence() *types.ProjectMCPDelivery {
	if a == nil || a.cfg == nil {
		return nil
	}
	return a.cfg.ProjectMCPDelivery
}

// projectMCPDelivers reports whether transport filtering removed a package the
// current execution materialized into its project configuration file.
func (a *Adapter) projectMCPDelivers(decision mcpServerFilterDecision) bool {
	if decision.Included || !transportUnsupportedReason(decision.ReasonCode) || !decision.Server.IsInjectedBuiltin() {
		return false
	}
	return validProjectMCPDelivery(a.projectDeliveryEvidence()).Delivers(decision.Server.BuiltinPackage)
}

func transportUnsupportedReason(reasonCode string) bool {
	return reasonCode == mcpFilterReasonSSEUnsupported || reasonCode == mcpFilterReasonHTTPUnsupported
}

// emitMCPDecisionEvidence publishes one safe decision per input server. A
// package delivered through the project file reports delivery once per package
// identity instead of an ACP transport failure.
func (a *Adapter) emitMCPDecisionEvidence(ctx context.Context, decisions []mcpServerFilterDecision) {
	projectEvidenced := make(map[p.PackageID]bool)
	for _, decision := range decisions {
		if a.projectMCPDelivers(decision) {
			if pkg := decision.Server.BuiltinPackage; !projectEvidenced[pkg] {
				projectEvidenced[pkg] = true
				a.emitMCPAttachmentEvidence(ctx, decision.Server, streams.MCPAttachmentEvidenceDelivered, projectMCPDeliveryReasonCode, "")
			}
			continue
		}
		kind := streams.MCPAttachmentEvidenceDelivered
		if !decision.Included {
			kind = streams.MCPAttachmentEvidenceFiltered
		}
		a.emitMCPAttachmentEvidence(ctx, decision.Server, kind, decision.ReasonCode, "")
	}
}
