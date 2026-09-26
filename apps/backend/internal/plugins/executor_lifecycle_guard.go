package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/internal/task/models"
)

// ErrExecutorProviderInventoryUnavailable reports that administrative guards
// cannot verify whether a plugin still owns remote executor resources.
var ErrExecutorProviderInventoryUnavailable = errors.New("plugins: executor provider inventory is unavailable")

// ExecutorProviderInventoryReader lists durable plugin-backed executor rows
// for lifecycle safety checks.
type ExecutorProviderInventoryReader interface {
	ListExecutorsRunningPluginRemote(context.Context) ([]*models.ExecutorRunning, error)
}

type retainedExecutorProviderInventory struct {
	PluginID               string `json:"plugin_id"`
	InstallationID         string `json:"installation_id"`
	ProviderKey            string `json:"provider_key"`
	ProviderIdentity       string `json:"provider_identity"`
	ContractVersion        int    `json:"contract_version"`
	SupportedStateVersions []int  `json:"supported_state_versions"`
	StateVersion           uint32 `json:"state_version"`
	Phase                  string `json:"phase"`
}

// SetExecutorProviderInventoryReader wires the durable runtime inventory used
// by administrative plugin lifecycle guards. It remains available when the
// provider feature gate is off because these checks must protect retained
// resources without dispatching plugin RPCs.
func (s *Service) SetExecutorProviderInventoryReader(reader ExecutorProviderInventoryReader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executorProviderInventoryReader = reader
}

func (s *Service) beginExecutorProviderDispatch(ctx context.Context, pluginID string) (context.Context, func(), error) {
	s.executorProviderOpMu.Lock()
	if s.executorProviderAdmissionClosed[pluginID] {
		s.executorProviderOpMu.Unlock()
		return nil, nil, ErrExecutorProviderUnavailable
	}
	if s.executorProviderDispatches == nil {
		s.executorProviderDispatches = make(map[string]map[uint64]context.CancelFunc)
	}
	s.executorProviderDispatchID++
	dispatchID := s.executorProviderDispatchID
	callCtx, cancel := context.WithCancel(ctx)
	if s.executorProviderDispatches[pluginID] == nil {
		s.executorProviderDispatches[pluginID] = make(map[uint64]context.CancelFunc)
	}
	s.executorProviderDispatches[pluginID][dispatchID] = cancel
	s.executorProviderOpMu.Unlock()
	return callCtx, func() {
		s.executorProviderOpMu.Lock()
		delete(s.executorProviderDispatches[pluginID], dispatchID)
		if len(s.executorProviderDispatches[pluginID]) == 0 {
			delete(s.executorProviderDispatches, pluginID)
		}
		s.executorProviderOpMu.Unlock()
		cancel()
	}, nil
}

// closeExecutorProviderAdmission rejects calls that have not yet acquired a
// dispatch lease and cancels the contexts of calls already admitted. The
// lifecycle caller then takes the exclusive dispatch lock to drain them.
func (s *Service) closeExecutorProviderAdmission(pluginID string) {
	s.executorProviderOpMu.Lock()
	if s.executorProviderAdmissionClosed == nil {
		s.executorProviderAdmissionClosed = make(map[string]bool)
	}
	s.executorProviderAdmissionClosed[pluginID] = true
	cancels := make([]context.CancelFunc, 0, len(s.executorProviderDispatches[pluginID]))
	for _, cancel := range s.executorProviderDispatches[pluginID] {
		cancels = append(cancels, cancel)
	}
	s.executorProviderOpMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (s *Service) openExecutorProviderAdmission(pluginID string) {
	s.executorProviderOpMu.Lock()
	delete(s.executorProviderAdmissionClosed, pluginID)
	s.executorProviderOpMu.Unlock()
}

func (s *Service) reopenExecutorProviderAdmissionIfActive(pluginID string) {
	record, err := s.Get(pluginID)
	if err == nil && record != nil && record.Status == StatusActive {
		s.openExecutorProviderAdmission(pluginID)
	}
}

func (s *Service) retainedExecutorProviderInventory(ctx context.Context, pluginID string) ([]retainedExecutorProviderInventory, error) {
	s.mu.Lock()
	reader := s.executorProviderInventoryReader
	s.mu.Unlock()
	if reader == nil {
		return nil, ErrExecutorProviderInventoryUnavailable
	}
	rows, err := reader.ListExecutorsRunningPluginRemote(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: inventory read failed", ErrExecutorProviderInventoryUnavailable)
	}
	retained := make([]retainedExecutorProviderInventory, 0)
	for _, row := range rows {
		if row == nil {
			return nil, fmt.Errorf("%w: inventory contains an empty row", ErrExecutorProviderInventoryUnavailable)
		}
		encoded, err := json.Marshal(row.Metadata["plugin_executor"])
		if err != nil || string(encoded) == "null" {
			return nil, fmt.Errorf("%w: inventory identity cannot be decoded", ErrExecutorProviderInventoryUnavailable)
		}
		var inventory retainedExecutorProviderInventory
		if err := json.Unmarshal(encoded, &inventory); err != nil || inventory.PluginID == "" || inventory.ProviderKey == "" {
			return nil, fmt.Errorf("%w: inventory identity is incomplete", ErrExecutorProviderInventoryUnavailable)
		}
		if inventory.PluginID != pluginID || inventory.Phase == "absent" {
			continue
		}
		retained = append(retained, inventory)
	}
	return retained, nil
}

func (s *Service) guardExecutorProviderUninstall(ctx context.Context, record *store.Record) error {
	if record == nil || len(record.ExecutorProviders) == 0 {
		return nil
	}
	retained, err := s.retainedExecutorProviderInventory(ctx, record.ID)
	if err != nil {
		return err
	}
	if len(retained) != 0 {
		return fmt.Errorf("plugins: cannot uninstall %q while retained or unresolved remote executor resources exist; re-enable the provider and use normal task cleanup, then retry uninstall", record.ID)
	}
	return nil
}

func (s *Service) guardExecutorProviderUpgrade(ctx context.Context, old *store.Record, candidate *manifest.Manifest) error {
	if old == nil || candidate == nil || (len(old.ExecutorProviders) == 0 && len(candidate.ExecutorProviders) == 0) {
		return nil
	}
	retained, err := s.retainedExecutorProviderInventory(ctx, candidate.ID)
	if err != nil {
		return err
	}
	for _, inventory := range retained {
		if err := validateExecutorProviderUpgradeInventory(old, candidate, inventory); err != nil {
			return err
		}
	}
	return nil
}

func validateExecutorProviderUpgradeInventory(old *store.Record, candidate *manifest.Manifest, inventory retainedExecutorProviderInventory) error {
	provider := executorProviderByKey(candidate.ExecutorProviders, inventory.ProviderKey)
	if provider == nil || inventory.ProviderIdentity != ExecutorProviderIdentity(candidate.ID, inventory.ProviderKey) {
		return incompatibleExecutorUpgrade(candidate.ID, inventory.ProviderKey, "provider identity")
	}
	if old.InstallationID == "" || inventory.InstallationID != old.InstallationID {
		return incompatibleExecutorUpgrade(candidate.ID, inventory.ProviderKey, "installation identity")
	}
	if provider.ContractVersion != inventory.ContractVersion {
		return incompatibleExecutorUpgrade(candidate.ID, inventory.ProviderKey, "contract version")
	}
	if inventory.StateVersion > 0 {
		if !supportsExecutorStateVersion(provider, int(inventory.StateVersion)) {
			return incompatibleExecutorUpgrade(candidate.ID, inventory.ProviderKey, fmt.Sprintf("state version %d", inventory.StateVersion))
		}
		return nil
	}
	// An interrupted allocation can still resolve to any state version the
	// previous provider declared. Keep every possible result readable until
	// the original operation has a known resource state.
	if len(inventory.SupportedStateVersions) == 0 {
		return incompatibleExecutorUpgrade(candidate.ID, inventory.ProviderKey, "unknown state version")
	}
	for _, stateVersion := range inventory.SupportedStateVersions {
		if !supportsExecutorStateVersion(provider, stateVersion) {
			return incompatibleExecutorUpgrade(candidate.ID, inventory.ProviderKey, fmt.Sprintf("state version %d", stateVersion))
		}
	}
	return nil
}

func executorProviderByKey(providers []manifest.ExecutorProvider, key string) *manifest.ExecutorProvider {
	for index := range providers {
		if providers[index].Key == key {
			return &providers[index]
		}
	}
	return nil
}

func supportsExecutorStateVersion(provider *manifest.ExecutorProvider, version int) bool {
	if provider == nil || version <= 0 {
		return false
	}
	for _, supported := range provider.SupportedStateVersions {
		if supported == version {
			return true
		}
	}
	return false
}

func incompatibleExecutorUpgrade(pluginID, providerKey, reason string) error {
	return fmt.Errorf("plugins: upgrade of %q is incompatible with retained provider %q (%s)", pluginID, providerKey, reason)
}
