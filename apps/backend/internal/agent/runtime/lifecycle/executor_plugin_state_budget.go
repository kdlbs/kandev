package lifecycle

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

const (
	pluginExecutorHostReserveBytes   = 20480
	pluginExecutorDefaultStateBudget = 8192
	pluginExecutorMaxStateBudget     = 12288
	// pluginExecutorEnvelopeFraming is the outer braces of the marshaled envelope less the absent last comma.
	pluginExecutorEnvelopeFraming = 1
)

// hostFieldGroup is one row of the host-written envelope budget. Maxima count
// the marshaled, JSON-escaped form of each field including its key and
// separator, so a value that escapes larger is rejected rather than silently
// consuming provider headroom.
type hostFieldGroup struct {
	name string
	max  int
	keys []string
}

// hostFieldBudget lists every envelope field the host writes. A field absent
// from the table fails TestEveryEnvelopeFieldHasAHostBudgetEntry.
var hostFieldBudget = []hostFieldGroup{
	{name: "existing", max: 5888, keys: []string{
		"plugin_id", "installation_id", "provider_key", "provider_identity", "environment_id", "contract_version",
		"supported_state_versions", "profile_id", "profile_config", "secret_references", "operation_id", "input_digest",
		"runtime_identity", "instance_port", "platform", "capabilities", "expires_at", "expiry_check_at",
		"last_inspection_state", "revision", "generation", "environment_generation", "resource",
	}},
	{name: "workspace_descriptor", max: 1792, keys: []string{"workspace_descriptor"}},
	{name: "negotiation", max: 1024, keys: []string{"negotiated_capabilities", "state_version", "profile_snapshot_digest", "workspace_preparation"}},
	{name: "repositories", max: 8192, keys: []string{"repositories"}},
	{name: "headroom", max: 2048, keys: nil},
	{name: "coordination", max: 1536, keys: []string{
		"phase", "placement", "admission", "launch_policy", "claim", "cleanup_requested", "handshake_started",
		"drain_outcome", "drain_receipt_id", "drain_claim", "publish_pending",
	}},
}

// checkPluginExecutorEnvelopeBudget enforces the per-field maxima and the
// provider state budget for an envelope that carries a negotiated capability
// set. A record without one keeps the legacy cap and error.
func checkPluginExecutorEnvelopeBudget(inventory pluginExecutorInventory, stateBudget int) error {
	if len(inventory.NegotiatedCapabilities) == 0 {
		return nil
	}
	if stateBudget <= 0 {
		stateBudget = pluginExecutorMaxStateBudget
	}
	stateBytes, err := escapedProviderStateBytes(inventory)
	if err != nil {
		return err
	}
	if stateBytes > stateBudget {
		return fmt.Errorf("%w: provider state is %d bytes, budget is %d", models.ErrExecutorStateTooLarge, stateBytes, stateBudget)
	}
	data, err := json.Marshal(inventory)
	if err != nil {
		return fmt.Errorf("%w: envelope cannot be encoded", models.ErrExecutorStateTooLarge)
	}
	if len(data) > pluginExecutorInventoryMaxBytes {
		return fmt.Errorf("%w: envelope is %d bytes, cap is %d", models.ErrExecutorStateTooLarge, len(data), pluginExecutorInventoryMaxBytes)
	}
	return checkHostFieldGroups(data, stateBytes)
}

// escapedProviderStateBytes measures the provider state as it appears inside
// the marshaled envelope, where escaping can inflate a byte up to six times.
func escapedProviderStateBytes(inventory pluginExecutorInventory) (int, error) {
	if inventory.Resource == nil || inventory.Resource.GetStateJson() == "" {
		return 0, nil
	}
	escaped, err := json.Marshal(inventory.Resource.GetStateJson())
	if err != nil {
		return 0, fmt.Errorf("%w: provider state cannot be encoded", models.ErrExecutorStateTooLarge)
	}
	return len(escaped) - 2, nil
}

func checkHostFieldGroups(data []byte, stateBytes int) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("%w: envelope cannot be measured", models.ErrExecutorStateTooLarge)
	}
	owner := map[string]string{}
	for _, group := range hostFieldBudget {
		for _, key := range group.keys {
			owner[key] = group.name
		}
	}
	used := map[string]int{"existing": pluginExecutorEnvelopeFraming}
	for key, value := range fields {
		group, known := owner[key]
		if !known {
			return fmt.Errorf("%w: envelope field %q has no budget", models.ErrExecutorStateTooLarge, key)
		}
		encodedKey, _ := json.Marshal(key)
		size := len(encodedKey) + 1 + len(value) + 1
		if key == "resource" {
			size -= stateBytes
		}
		used[group] += size
	}
	for _, group := range hostFieldBudget {
		if used[group.name] > group.max {
			return fmt.Errorf("%w: %s fields are %d bytes, maximum is %d", models.ErrExecutorStateTooLarge, group.name, used[group.name], group.max)
		}
	}
	return nil
}

// fixedSizePluginExecutorResource keeps only what cleanup needs to find the
// resource again, so recording a refused response cannot itself exceed a cap.
func fixedSizePluginExecutorResource(resource *pluginsdk.ExecutorResourceDescriptor) *pluginsdk.ExecutorResourceDescriptor {
	if resource == nil {
		return nil
	}
	return &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: resource.GetResourceHandle(), StateVersion: resource.GetStateVersion()}
}

// withPluginExecutorLaunchCode keeps a host-originated stable code visible
// through the generic launch-failure wording.
func withPluginExecutorLaunchCode(cause, failure error) error {
	for _, code := range []error{models.ErrExecutorStateTooLarge, models.ErrExecutorCapabilityMismatch, models.ErrExecutorSecretInState, models.ErrExecutorConflict} {
		if errors.Is(cause, code) {
			return fmt.Errorf("%w: %v", code, failure)
		}
	}
	return failure
}

// pluginExecutorStateBudget is the manifest's provider-state budget, or the default.
func pluginExecutorStateBudget(provider models.ExecutorProvider) int {
	if provider.StateBudgetBytes > 0 {
		return provider.StateBudgetBytes
	}
	return pluginExecutorDefaultStateBudget
}
