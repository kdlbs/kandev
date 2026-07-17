package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

	// configSchema is the plugin's manifest config_schema, used by
	// GetConfig to know which fields are secret (and therefore stored as
	// vault references to resolve back to cleartext).
	configSchema map[string]any

	state   *state.Store
	secrets SecretVault
	bus     bus.EventBus

	// configs reads the plugin's operator-editable config for the ungated
	// GetConfig RPC. Satisfied by store.Store; nil in tests that build a
	// pluginHost without one (GetConfig then returns an empty map).
	configs configReader

	// Host data API (ADR 0043) service-layer dependencies, wired by
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

// taskNotFound builds the gRPC error taskReader.Get returns for an id that
// doesn't resolve to a task — the SAME error, in-process or over the wire
// (grpcHostServer.GetTask forwards it as-is), so a plugin never observes a
// (nil, nil) success for a missing task.
func taskNotFound(id string) error {
	return status.Errorf(codes.NotFound, "task %q not found", id)
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

// configReader is the narrow slice of store.Store pluginHost needs for the
// GetConfig RPC.
type configReader interface {
	GetConfig(id string) (map[string]any, error)
}

// GetConfig returns the plugin's own operator-editable config, secret values
// included — this RPC is how a configured credential (e.g. a PAT) reaches
// the plugin process. Secret fields are stored as vault references
// (configVaultRef) and resolved back to cleartext here; this resolution is
// deliberately NOT gated on capabilities.secrets, since it is the plugin's
// own config, set by the operator specifically for it.
func (h *pluginHost) GetConfig(ctx context.Context) (map[string]any, error) {
	if h.configs == nil {
		return map[string]any{}, nil
	}
	config, err := h.configs.GetConfig(h.pluginID)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return map[string]any{}, nil
	}
	return h.resolveConfigSecrets(ctx, config)
}

// resolveConfigSecrets replaces each secret config field's vault reference
// with the cleartext value from the vault. A resolution failure is an error
// (not a silent drop): the plugin must never mistake a broken vault for
// "the operator cleared this setting". Works on a copy — writing cleartext
// into the caller's map would leak it into any future cached configReader
// value, even though today's FSStore.GetConfig always returns a fresh map.
func (h *pluginHost) resolveConfigSecrets(ctx context.Context, config map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(config))
	for k, v := range config {
		out[k] = v
	}
	for field := range secretPropertyKeys(h.configSchema) {
		if !isConfigVaultRef(h.pluginID, field, out[field]) {
			continue
		}
		if h.secrets == nil {
			return nil, errors.New("plugins: secret vault not configured")
		}
		cleartext, err := h.secrets.Reveal(ctx, pluginConfigSecretID(h.pluginID, field))
		if err != nil {
			return nil, fmt.Errorf("plugins: resolve secret config field %q: %w", field, err)
		}
		out[field] = cleartext
	}
	return out, nil
}

// GetSecret reads a plugin-owned secret previously stored via SetSecret.
// found is false (with a nil error) when the key was never set. Requires
// capabilities.secrets, like every secret-touching RPC.
func (h *pluginHost) GetSecret(ctx context.Context, key string) (string, bool, error) {
	if err := h.checkSecretAccess(key); err != nil {
		return "", false, err
	}
	value, err := h.secrets.Reveal(ctx, pluginSecretID(h.pluginID, key))
	if err != nil {
		if strings.HasPrefix(err.Error(), "secret not found:") {
			return "", false, nil
		}
		return "", false, err
	}
	return value, true, nil
}

// SetSecret upserts a plugin-owned secret into kandev's encrypted vault,
// namespaced to this plugin (vault id "plugin:<id>:secret:<key>") — a
// plugin can never write outside its own namespace.
func (h *pluginHost) SetSecret(ctx context.Context, key, value string) error {
	if err := h.checkSecretAccess(key); err != nil {
		return err
	}
	vaultID := pluginSecretID(h.pluginID, key)
	return h.secrets.Set(ctx, vaultID, vaultID, value)
}

// DeleteSecret removes a plugin-owned secret. Deleting a missing key is not
// an error, matching DeleteState's semantics.
func (h *pluginHost) DeleteSecret(ctx context.Context, key string) error {
	if err := h.checkSecretAccess(key); err != nil {
		return err
	}
	err := h.secrets.Delete(ctx, pluginSecretID(h.pluginID, key))
	if err != nil && strings.HasPrefix(err.Error(), "secret not found:") {
		return nil
	}
	return err
}

// checkSecretAccess is the shared gate for the plugin-scoped secret RPCs:
// capability secrets declared, a wired vault, and a key that is a single
// sane identifier (pluginSecretKeyPattern) so it can never smuggle
// separators into the vault-id namespace.
func (h *pluginHost) checkSecretAccess(key string) error {
	if !h.capabilities.Secrets {
		return permissionDenied("secrets")
	}
	if h.secrets == nil {
		return errors.New("plugins: secret vault not configured")
	}
	if !pluginSecretKeyPattern.MatchString(key) {
		return status.Errorf(codes.InvalidArgument, "invalid secret key %q", key)
	}
	return nil
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
