package plugins

import (
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kdlbs/kandev/pluginsdk"
)

type NotificationProviderAccess interface {
	NotificationProviderType(context.Context, string, string) (models.ProviderType, error)
	NotificationFormatterBinding(context.Context, string, string) (*models.FormatterBinding, error)
	BeginFormatterManagement(context.Context) (func(), error)
}

// CaptureFormatterLifecycle holds installation read leases while the operator
// captures native SQLite snapshots. Rendering may continue against immutable data.
func (s *Service) CaptureFormatterLifecycle(ctx context.Context) (func(), error) {
	var releases []func()
	var once sync.Once
	release := func() {
		once.Do(func() {
			for index := len(releases) - 1; index >= 0; index-- {
				releases[index]()
			}
		})
	}
	for _, record := range s.registry.List() {
		if len(record.NotificationFormatters) == 0 {
			continue
		}
		unlock, err := s.dispatchLocks.lockFor(record.ID).RLockContext(ctx)
		if err != nil {
			release()
			return nil, err
		}
		releases = append(releases, unlock)
		current, err := s.Get(record.ID)
		if err != nil || current.InstallationID != record.InstallationID || !dispatchGeneration(record).matches(current) {
			release()
			return nil, errors.New("notification formatter installation changed")
		}
	}
	return release, nil
}

func (s *Service) SetNotificationProviderAccess(access NotificationProviderAccess) {
	s.notificationProviders = access
}

func (s *Service) NotificationFormatters(ctx context.Context, kind models.ProviderType) ([]models.FormatterDescriptor, error) {
	result := []models.FormatterDescriptor{}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, record := range s.registry.List() {
		if record.Status != StatusActive || record.InstallationID == "" {
			continue
		}
		for _, declared := range record.NotificationFormatters {
			if declared.ContractVersion == 1 && slices.Contains(declared.ProviderTypes, string(kind)) {
				result = append(result, models.FormatterDescriptor{PluginID: record.ID, InstallationID: record.InstallationID, Key: declared.Key, Version: record.Version})
			}
		}
	}
	return result, nil
}

func (s *Service) formatterLease(ctx context.Context, owner string, binding *models.FormatterBinding) (*pluginsdk.RemotePlugin, func(), error) {
	if binding == nil || s.notificationProviders == nil {
		return nil, nil, errors.New("notification formatter unavailable")
	}
	kind, err := s.notificationProviders.NotificationProviderType(ctx, owner, binding.ProviderID)
	if err != nil || kind != binding.ProviderType {
		return nil, nil, errors.New("notification formatter provider unavailable")
	}
	record, err := s.Get(binding.PluginID)
	if err != nil || record.InstallationID != binding.InstallationID {
		return nil, nil, errors.New("notification formatter installation unavailable")
	}
	if _, ok := declaredNotificationFormatter(record, binding.FormatterKey, kind, ""); !ok {
		return nil, nil, errors.New("notification formatter not declared")
	}
	_, release, err := s.beginPluginDispatchContext(ctx, binding.PluginID, dispatchGeneration(record))
	if err != nil {
		return nil, nil, err
	}
	remote, ok := s.pluginRemote(binding.PluginID)
	if !ok {
		release()
		return nil, nil, errors.New("notification formatter not running")
	}
	return remote, release, nil
}

func (s *Service) FormatNotification(ctx context.Context, binding *models.FormatterBinding, req *pluginsdk.NotificationFormatRequest) (*pluginsdk.NotificationFormatResponse, error) {
	if req == nil || binding == nil || req.ProviderId != binding.ProviderID || req.FormatterKey != binding.FormatterKey || req.ConfigurationRevision != binding.ConfigurationRevision {
		return nil, errors.New("invalid notification formatter context")
	}
	remote, release, err := s.formatterLease(ctx, req.OwnerId, binding)
	if err != nil {
		return nil, err
	}
	defer release()
	return remote.FormatNotification(ctx, req)
}

// LeaseFormatterConfiguration retains installation identity until the caller
// finishes committing its binding. The caller must release every successful lease.
func (s *Service) LeaseFormatterConfiguration(ctx context.Context, owner string, binding *models.FormatterBinding) (*pluginsdk.NotificationConfigurationResponse, func(), error) {
	remote, release, err := s.formatterLease(ctx, owner, binding)
	if err != nil {
		return nil, nil, err
	}
	receipt, err := remote.InspectNotificationConfiguration(ctx, &pluginsdk.NotificationConfigurationRequest{
		OwnerId: owner, ProviderId: binding.ProviderID, FormatterKey: binding.FormatterKey, Revision: binding.ConfigurationRevision,
	})
	if err != nil {
		release()
		return nil, nil, err
	}
	return receipt, release, nil
}

func declaredNotificationFormatter(record *store.Record, key string, kind models.ProviderType, action string) (manifest.NotificationFormatter, bool) {
	for _, declared := range record.NotificationFormatters {
		if declared.ContractVersion == 1 && (key == "" || declared.Key == key) && slices.Contains(declared.ProviderTypes, string(kind)) && (action == "" || slices.Contains(declared.ManagementActions, action)) {
			return declared, true
		}
	}
	return manifest.NotificationFormatter{}, false
}
