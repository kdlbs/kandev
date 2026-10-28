package lifecycle

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/plugins/profilesnapshot"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// checkRetainedPluginExecutorProfileSize applies the aggregate profile limit
// to a resume only when the environment was allocated with a non-empty
// negotiated set, so a later plugin upgrade cannot fail an older environment.
func checkRetainedPluginExecutorProfileSize(inventory pluginExecutorInventory, profile *models.ExecutorProviderLaunchProfile) error {
	if len(inventory.NegotiatedCapabilities) == 0 || profile == nil {
		return nil
	}
	ids := make([]string, 0, len(profile.SecretReferences))
	for _, id := range profile.SecretReferences {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if models.ExecutorProfileContractBytes(profile.Config, ids) > models.ExecutorProfileContractBudgetBytes {
		return fmt.Errorf("%w: provider profile exceeds the size limit", models.ErrExecutorProfileTooLarge)
	}
	return nil
}

// pluginExecutorProfileSnapshot builds the provider-facing snapshot for one
// operation from the negotiated `name@version` set that governs the
// environment. An empty set yields the legacy snapshot.
func pluginExecutorProfileSnapshot(profile models.ExecutorProviderLaunchProfile, negotiated []string) (*pluginsdk.ExecutorProfileSnapshot, error) {
	names := make([]string, 0, len(negotiated))
	for _, pair := range negotiated {
		name, _, _ := strings.Cut(pair, "@")
		names = append(names, name)
	}
	snapshot, err := profilesnapshot.Build(profilesnapshot.Input{
		ProfileID: profile.ProfileID, Config: profile.Config, SecretValues: profile.SecretValues,
		SecretRefs: profile.SecretReferences, Schema: profile.Provider.ProfileSchema, Capabilities: names,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %s", models.ErrExecutorProfileInvalidType, err.Error())
	}
	return snapshot, nil
}

// pluginExecutorSecretScanMinBytes is the shortest revealed value that is
// scanned for in provider state; shorter values would match by coincidence.
const pluginExecutorSecretScanMinBytes = 8

// rejectSecretInProviderState refuses a response whose state contains the
// exact bytes of a secret value revealed for the call, in raw or JSON-escaped
// form. Transformed copies are a provider obligation and are not detected. The
// error names no value.
func rejectSecretInProviderState(secretValues map[string]string, resource *pluginsdk.ExecutorResourceDescriptor) error {
	state := resource.GetStateJson()
	if state == "" {
		return nil
	}
	for _, value := range secretValues {
		if len(value) < pluginExecutorSecretScanMinBytes {
			continue
		}
		if strings.Contains(state, value) || strings.Contains(state, jsonEscapedText(value)) {
			return fmt.Errorf("%w: provider state contains a revealed secret value", models.ErrExecutorSecretInState)
		}
	}
	return nil
}

func jsonEscapedText(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) < 2 {
		return value
	}
	return string(encoded[1 : len(encoded)-1])
}
