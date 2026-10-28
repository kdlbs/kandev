package plugins

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/internal/task/models"
)

// ExecutorStateReadabilityLocker is implemented by the durable inventory. The
// lock excludes an admission write that records a provider state version from
// an activation that counts the recorded versions.
type ExecutorStateReadabilityLocker interface {
	WithPluginStateReadabilityLock(ctx context.Context, installationID string, fn func(context.Context) error) error
}

// guardExecutorProviderUpgradeLocked runs the recorded-state count under the
// installation's readability lock and, when it passes, publishes the
// candidate's readable lists so an admission that commits afterwards is
// refused. The caller clears the override once the manifest switch settles.
func (s *Service) guardExecutorProviderUpgradeLocked(ctx context.Context, old *store.Record, candidate *manifest.Manifest) error {
	guard := func(ctx context.Context) error {
		if err := s.guardExecutorProviderUpgrade(ctx, old, candidate); err != nil {
			return err
		}
		s.setReadableOverride(old.InstallationID, readableStateLists(candidate.ExecutorProviders))
		return nil
	}
	s.mu.Lock()
	reader := s.executorProviderInventoryReader
	s.mu.Unlock()
	locker, ok := reader.(ExecutorStateReadabilityLocker)
	if !ok || old.InstallationID == "" {
		return guard(ctx)
	}
	return locker.WithPluginStateReadabilityLock(ctx, old.InstallationID, guard)
}

func readableStateLists(providers []manifest.ExecutorProvider) map[string][]int {
	lists := make(map[string][]int, len(providers))
	for _, provider := range providers {
		lists[provider.Key] = append([]int(nil), provider.SupportedStateVersions...)
	}
	return lists
}

func (s *Service) setReadableOverride(installationID string, lists map[string][]int) {
	s.stateReadableMu.Lock()
	defer s.stateReadableMu.Unlock()
	if s.stateReadableOverride == nil {
		s.stateReadableOverride = make(map[string]map[string][]int)
	}
	s.stateReadableOverride[installationID] = lists
}

func (s *Service) clearReadableOverride(installationID string) {
	s.stateReadableMu.Lock()
	defer s.stateReadableMu.Unlock()
	delete(s.stateReadableOverride, installationID)
}

// CheckExecutorStateWritable reports whether the activated manifest of an
// installation (or the candidate of an upgrade whose count already passed) can
// read a provider state version. Every unknown input fails closed with the
// stable capability_mismatch code.
func (s *Service) CheckExecutorStateWritable(_ context.Context, installationID, providerKey string, version uint32) error {
	s.stateReadableMu.Lock()
	lists, overridden := s.stateReadableOverride[installationID]
	s.stateReadableMu.Unlock()
	if !overridden {
		lists = s.activeReadableLists(installationID)
	}
	for _, readable := range lists[providerKey] {
		if readable > 0 && uint32(readable) == version {
			return nil
		}
	}
	return fmt.Errorf("%w: state version is not readable by the installed provider", models.ErrExecutorCapabilityMismatch)
}

func (s *Service) activeReadableLists(installationID string) map[string][]int {
	for _, record := range s.List() {
		if record.InstallationID == installationID {
			return readableStateLists(record.ExecutorProviders)
		}
	}
	return nil
}
