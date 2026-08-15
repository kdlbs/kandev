package agents

import (
	"context"

	"github.com/kandev/kandev/internal/agent/usage"
)

const (
	// DynamicAgentID is the stable registry and settings-family identity for
	// dynamic profiles. It is also used as the durable parent-agent row ID.
	DynamicAgentID = "dynamic"

	dynamicAgentName        = "Dynamic"
	dynamicAgentDescription = "Routes one logical session through ordered concrete agent profiles."
)

var _ VirtualAgent = (*DynamicAgent)(nil)

// DynamicAgent is the built-in virtual family for dynamic profiles. It is
// intentionally not an InferenceAgent: a dynamic profile resolves a concrete
// candidate before any launch or probe occurs.
type DynamicAgent struct{}

func NewDynamicAgent() *DynamicAgent { return &DynamicAgent{} }

func (a *DynamicAgent) ID() string              { return DynamicAgentID }
func (a *DynamicAgent) Name() string            { return dynamicAgentName }
func (a *DynamicAgent) DisplayName() string     { return dynamicAgentName }
func (a *DynamicAgent) Description() string     { return dynamicAgentDescription }
func (a *DynamicAgent) Enabled() bool           { return true }
func (a *DynamicAgent) DisplayOrder() int       { return -1 }
func (a *DynamicAgent) Logo(LogoVariant) []byte { return nil }

func (a *DynamicAgent) IsInstalled(context.Context) (*DiscoveryResult, error) {
	return nil, ErrNotSupported
}

func (a *DynamicAgent) BuildCommand(CommandOptions) Command { return Command{} }

func (a *DynamicAgent) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

func (a *DynamicAgent) Runtime() *RuntimeConfig { return nil }

func (a *DynamicAgent) BillingType() usage.BillingType { return usage.BillingTypeAPIKey }

func (a *DynamicAgent) RemoteAuth() *RemoteAuth { return nil }

func (a *DynamicAgent) InstallScript() string { return "" }

func (a *DynamicAgent) IsVirtual() bool { return true }
