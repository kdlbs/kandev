package agents

import (
	"context"

	"github.com/kandev/kandev/internal/agent/usage"
)

const CursorCloudAgentID = "cursor_cloud"

// ManagedRemoteAgent marks an agent family whose inference runtime is owned
// by a remote provider and must never be launched through the local CLI path.
type ManagedRemoteAgent interface {
	Agent
	ManagedRemoteRuntime() string
}

var _ VirtualAgent = (*CursorCloudAgent)(nil)
var _ ManagedRemoteAgent = (*CursorCloudAgent)(nil)

// CursorCloudAgent is a settings identity only. Dispatch is implemented by
// the managed runtime router, never by BuildCommand or local discovery.
type CursorCloudAgent struct{}

func NewCursorCloudAgent() *CursorCloudAgent { return &CursorCloudAgent{} }

func (*CursorCloudAgent) ID() string              { return CursorCloudAgentID }
func (*CursorCloudAgent) Name() string            { return "Cursor Cloud" }
func (*CursorCloudAgent) DisplayName() string     { return "Cursor Cloud" }
func (*CursorCloudAgent) Description() string     { return "Runs coding tasks in Cursor Cloud." }
func (*CursorCloudAgent) Enabled() bool           { return true }
func (*CursorCloudAgent) DisplayOrder() int       { return 100 }
func (*CursorCloudAgent) Logo(LogoVariant) []byte { return nil }
func (*CursorCloudAgent) IsInstalled(context.Context) (*DiscoveryResult, error) {
	return nil, ErrNotSupported
}
func (*CursorCloudAgent) BuildCommand(CommandOptions) Command { return Command{} }
func (*CursorCloudAgent) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}
func (*CursorCloudAgent) Runtime() *RuntimeConfig        { return nil }
func (*CursorCloudAgent) BillingType() usage.BillingType { return usage.BillingTypeAPIKey }
func (*CursorCloudAgent) RemoteAuth() *RemoteAuth        { return nil }
func (*CursorCloudAgent) InstallScript() string          { return "" }
func (*CursorCloudAgent) IsVirtual() bool                { return true }
func (*CursorCloudAgent) ManagedRemoteRuntime() string   { return "cursor_cloud" }
