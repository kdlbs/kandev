package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// pluginHost implements pluginsdk.Host (the kandev.plugin.v1 Host RPCs, §3
// of docs/plans/plugins/GRPC-CONTRACT.md) for exactly one plugin. Service
// hands runtime.Manager a fresh pluginHost per spawn/restart via
// hostForPlugin, bound to that plugin's id and its manifest-declared
// capabilities at spawn time. Every method is capability-gated: State
// methods (GetState/SetState/DeleteState/ListState) require
// capabilities.state, RevealSecret requires capabilities.secrets. EmitEvent
// is intentionally ungated — the frozen contract's capability-gating list
// (§5) only names state and secrets (api_read/api_write reserved for
// future); event emission has no boolean capability to gate on.
type pluginHost struct {
	// UnimplementedHostData is embedded so pluginHost satisfies
	// pluginsdk.Host even when one of the data-source fields below is nil
	// (e.g. a test pluginHost built without SetDataSources' wiring, or a
	// capability the manifest doesn't declare — see host_data.go's denied
	// readers for the capability-gated path). Every accessor
	// (Tasks/Sessions/Workspaces/Workflows/AgentProfiles/Repositories) is
	// overridden with a real, capability-gated implementation in
	// host_data.go; this embed only remains as defense-in-depth.
	pluginsdk.UnimplementedHostData

	pluginID     string
	capabilities manifest.Capabilities

	state   *state.Store
	secrets SecretRevealer
	bus     bus.EventBus

	// Host data API (ADR 0042) service-layer dependencies, wired by
	// Service.hostForPlugin from Service.SetDataSources. See host_data.go.
	taskData         taskDataSource
	workflows        workflowLister
	workflowSteps    workflowStepLister
	agentProfiles    agentProfileDataSource
	sessionCodeStats sessionCodeStatsSource
}

var _ pluginsdk.Host = (*pluginHost)(nil)

// permissionDenied builds the gRPC error RemotePlugin/Host RPCs return for
// an undeclared capability, matching the wire-level message from
// docs/specs/plugins/spec.md ("Permissions"): "capability '<name>' not
// declared".
func permissionDenied(capability string) error {
	return status.Errorf(codes.PermissionDenied, "capability '%s' not declared", capability)
}

func (h *pluginHost) GetState(ctx context.Context, scope, scopeID, key string) (map[string]any, bool, error) {
	if !h.capabilities.State {
		return nil, false, permissionDenied("state")
	}
	raw, found, err := h.state.Get(ctx, h.pluginID, scope, scopeID, key)
	if err != nil || !found {
		return nil, found, err
	}
	value, err := unmarshalStateValue(raw)
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func (h *pluginHost) SetState(ctx context.Context, scope, scopeID, key string, value map[string]any) error {
	if !h.capabilities.State {
		return permissionDenied("state")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("plugins: marshal state value: %w", err)
	}
	return h.state.Set(ctx, h.pluginID, scope, scopeID, key, raw)
}

func (h *pluginHost) DeleteState(ctx context.Context, scope, scopeID, key string) error {
	if !h.capabilities.State {
		return permissionDenied("state")
	}
	return h.state.Delete(ctx, h.pluginID, scope, scopeID, key)
}

func (h *pluginHost) ListState(ctx context.Context, scope, scopeID string) ([]pluginsdk.StateEntry, error) {
	if !h.capabilities.State {
		return nil, permissionDenied("state")
	}
	entries, err := h.state.List(ctx, h.pluginID, scope, scopeID)
	if err != nil {
		return nil, err
	}
	out := make([]pluginsdk.StateEntry, len(entries))
	for i, e := range entries {
		value, err := unmarshalStateValue(e.Value)
		if err != nil {
			return nil, err
		}
		out[i] = pluginsdk.StateEntry{
			Key:       e.Key,
			Value:     value,
			UpdatedAt: e.UpdatedAt.UTC().Format(time.RFC3339),
		}
	}
	return out, nil
}

func (h *pluginHost) RevealSecret(ctx context.Context, ref string) (string, error) {
	if !h.capabilities.Secrets {
		return "", permissionDenied("secrets")
	}
	if h.secrets == nil {
		return "", errors.New("plugins: secret vault not configured")
	}
	return h.secrets.Reveal(ctx, ref)
}

// EmitEvent publishes a plugin-originated event onto the bus, subject
// "plugin.<id>.<name>" (per the task's build instructions). A no-op if no
// event bus was wired (e.g. early boot, or a test Service without one).
func (h *pluginHost) EmitEvent(ctx context.Context, name string, payload map[string]any) error {
	if h.bus == nil {
		return nil
	}
	subject := "plugin." + h.pluginID + "." + name
	event := bus.NewEvent(subject, "plugin:"+h.pluginID, payload)
	return h.bus.Publish(ctx, subject, event)
}

// unmarshalStateValue decodes a plugin_state row's JSON value into a
// Go-native map, matching pluginsdk's Struct<->map[string]any convention.
func unmarshalStateValue(raw json.RawMessage) (map[string]any, error) {
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("plugins: unmarshal state value: %w", err)
	}
	return value, nil
}
