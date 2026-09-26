package plugins

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/protobuf/proto"
)

const executorProviderAvailabilityFeatureDisabled = "feature_disabled"

var (
	ErrExecutorProvidersDisabled   = errors.New("plugins: remote executor providers are disabled")
	ErrExecutorProviderUnavailable = errors.New("plugins: executor provider is unavailable")
)

type executorProviderContractProbe interface {
	ValidateExecutorProfile(context.Context, *pluginsdk.ValidateExecutorProfileRequest) (*pluginsdk.ValidateExecutorProfileResponse, error)
}

// ExecutorProviderIdentity returns the host-owned provider identity.
func ExecutorProviderIdentity(pluginID, key string) string {
	return "plugin:" + pluginID + ":" + key
}

// ListExecutorProviders returns manifest-owned executor entries, including
// unavailable declarations so saved selections can remain visible.
func (s *Service) ListExecutorProviders(ctx context.Context) ([]models.ExecutorProvider, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	enabled := s.remoteExecutorProvidersEnabled()
	providers := make([]models.ExecutorProvider, 0)
	for _, record := range s.List() {
		for _, declared := range record.ExecutorProviders {
			identity := ExecutorProviderIdentity(record.ID, declared.Key)
			localizedMessages := make(map[string]string, len(declared.LocalizedMessages))
			for key, message := range declared.LocalizedMessages {
				localizedMessages[key] = message
			}
			entry := models.ExecutorProvider{
				ExecutorID:             models.ExecutorProviderExecutorID(identity),
				Identity:               identity,
				PluginID:               record.ID,
				InstallationID:         record.InstallationID,
				Key:                    declared.Key,
				ContractVersion:        declared.ContractVersion,
				SupportedStateVersions: append([]int(nil), declared.SupportedStateVersions...),
				DisplayName:            declared.DisplayName,
				Description:            declared.Description,
				LocalizedMessages:      localizedMessages,
				ProfileSchema:          declared.ProfileSchema,
				ResourceStateSchema:    declared.ResourceStateSchema,
				Capabilities: models.ExecutorProviderCapabilities{
					Terminal:            declared.Capabilities.Terminal,
					Files:               declared.Capabilities.Files,
					Git:                 declared.Capabilities.Git,
					EmbeddedEditor:      declared.Capabilities.EmbeddedEditor,
					Preview:             declared.Capabilities.Preview,
					Reattach:            declared.Capabilities.Reattach,
					Retention:           declared.Capabilities.Retention,
					MaximumLifetimeSecs: declared.Capabilities.MaximumLifetimeSecs,
				},
			}
			switch {
			case !enabled:
				entry.AvailabilityCause = executorProviderAvailabilityFeatureDisabled
			case record.Status != StatusActive:
				entry.AvailabilityCause = "plugin_disabled"
			default:
				if _, running := s.pluginRemote(record.ID); !running {
					entry.AvailabilityCause = "plugin_unavailable"
				} else {
					entry.Available = true
				}
			}
			providers = append(providers, entry)
		}
	}
	return providers, nil
}

// ValidateExecutorProviderProfile sends only the selected provider's profile
// snapshot to the plugin. The host supplies transient secret values separately
// and never returns them in the validation result.
func (s *Service) ValidateExecutorProviderProfile(ctx context.Context, executorID string, profile models.ExecutorProviderProfile) ([]models.ExecutorProviderFieldError, error) {
	providers, err := s.ListExecutorProviders(ctx)
	if err != nil {
		return nil, err
	}
	var selected *models.ExecutorProvider
	for index := range providers {
		if providers[index].ExecutorID == executorID {
			selected = &providers[index]
			break
		}
	}
	if selected == nil || !selected.Available {
		return nil, ErrExecutorProviderUnavailable
	}
	var fieldErrors []models.ExecutorProviderFieldError
	err = s.withExecutorProvider(ctx, selected.PluginID, selected.Key, func(callCtx context.Context, remote *pluginsdk.RemotePlugin, declaration *manifest.ExecutorProvider) error {
		response, callErr := remote.ValidateExecutorProfile(callCtx, &pluginsdk.ValidateExecutorProfileRequest{
			Context: &pluginsdk.ExecutorProviderRequestContext{
				PluginId: selected.PluginID, InstallationId: selected.InstallationID,
				ProviderKey: selected.Key, ContractVersion: int32(declaration.ContractVersion),
			},
			Profile: &pluginsdk.ExecutorProfileSnapshot{
				ProfileId: profile.ProfileID, Config: profile.Config, SecretValues: profile.SecretValues,
			},
		})
		if callErr != nil {
			return fmt.Errorf("%w: provider profile validation failed", ErrExecutorProviderUnavailable)
		}
		if response == nil {
			return fmt.Errorf("%w: provider returned an empty profile validation response", ErrExecutorProviderUnavailable)
		}
		for _, item := range response.FieldErrors {
			if item == nil {
				continue
			}
			fieldErrors = append(fieldErrors, models.ExecutorProviderFieldError{Field: item.Field, Code: item.Code, MessageID: item.MessageId})
		}
		if response.Error != nil {
			return fmt.Errorf("%w: provider rejected profile with code %q", ErrExecutorProviderUnavailable, response.Error.Code)
		}
		return nil
	})
	return fieldErrors, err
}

func (s *Service) ProvisionExecutorEnvironment(ctx context.Context, req *pluginsdk.ProvisionExecutorEnvironmentRequest) (*pluginsdk.ProvisionExecutorEnvironmentResponse, error) {
	var response *pluginsdk.ProvisionExecutorEnvironmentResponse
	err := s.invokeExecutorProvider(ctx, req.GetContext(), func(callCtx context.Context, remote *pluginsdk.RemotePlugin, trusted *pluginsdk.ExecutorProviderRequestContext) error {
		copy := proto.Clone(req).(*pluginsdk.ProvisionExecutorEnvironmentRequest)
		copy.Context = trusted
		var callErr error
		response, callErr = remote.ProvisionExecutorEnvironment(callCtx, copy)
		return callErr
	})
	return response, err
}

func (s *Service) RecoverExecutorOperation(ctx context.Context, req *pluginsdk.RecoverExecutorOperationRequest) (*pluginsdk.RecoverExecutorOperationResponse, error) {
	var response *pluginsdk.RecoverExecutorOperationResponse
	err := s.invokeExecutorProvider(ctx, req.GetContext(), func(callCtx context.Context, remote *pluginsdk.RemotePlugin, trusted *pluginsdk.ExecutorProviderRequestContext) error {
		copy := proto.Clone(req).(*pluginsdk.RecoverExecutorOperationRequest)
		copy.Context = trusted
		var callErr error
		response, callErr = remote.RecoverExecutorOperation(callCtx, copy)
		return callErr
	})
	return response, err
}

func (s *Service) AttachExecutorEnvironment(ctx context.Context, req *pluginsdk.AttachExecutorEnvironmentRequest) (*pluginsdk.AttachExecutorEnvironmentResponse, error) {
	var response *pluginsdk.AttachExecutorEnvironmentResponse
	err := s.invokeExecutorProvider(ctx, req.GetContext(), func(callCtx context.Context, remote *pluginsdk.RemotePlugin, trusted *pluginsdk.ExecutorProviderRequestContext) error {
		copy := proto.Clone(req).(*pluginsdk.AttachExecutorEnvironmentRequest)
		copy.Context = trusted
		var callErr error
		response, callErr = remote.AttachExecutorEnvironment(callCtx, copy)
		return callErr
	})
	return response, err
}

func (s *Service) InspectExecutorEnvironment(ctx context.Context, req *pluginsdk.InspectExecutorEnvironmentRequest) (*pluginsdk.InspectExecutorEnvironmentResponse, error) {
	var response *pluginsdk.InspectExecutorEnvironmentResponse
	err := s.invokeExecutorProvider(ctx, req.GetContext(), func(callCtx context.Context, remote *pluginsdk.RemotePlugin, trusted *pluginsdk.ExecutorProviderRequestContext) error {
		copy := proto.Clone(req).(*pluginsdk.InspectExecutorEnvironmentRequest)
		copy.Context = trusted
		var callErr error
		response, callErr = remote.InspectExecutorEnvironment(callCtx, copy)
		return callErr
	})
	return response, err
}

func (s *Service) ResolveExecutorConnection(ctx context.Context, req *pluginsdk.ResolveExecutorConnectionRequest) (*pluginsdk.ResolveExecutorConnectionResponse, error) {
	var response *pluginsdk.ResolveExecutorConnectionResponse
	err := s.invokeExecutorProvider(ctx, req.GetContext(), func(callCtx context.Context, remote *pluginsdk.RemotePlugin, trusted *pluginsdk.ExecutorProviderRequestContext) error {
		copy := proto.Clone(req).(*pluginsdk.ResolveExecutorConnectionRequest)
		copy.Context = trusted
		var callErr error
		response, callErr = remote.ResolveExecutorConnection(callCtx, copy)
		return callErr
	})
	return response, err
}

func (s *Service) DestroyExecutorEnvironment(ctx context.Context, req *pluginsdk.DestroyExecutorEnvironmentRequest) (*pluginsdk.DestroyExecutorEnvironmentResponse, error) {
	var response *pluginsdk.DestroyExecutorEnvironmentResponse
	err := s.invokeExecutorProvider(ctx, req.GetContext(), func(callCtx context.Context, remote *pluginsdk.RemotePlugin, trusted *pluginsdk.ExecutorProviderRequestContext) error {
		copy := proto.Clone(req).(*pluginsdk.DestroyExecutorEnvironmentRequest)
		copy.Context = trusted
		var callErr error
		response, callErr = remote.DestroyExecutorEnvironment(callCtx, copy)
		return callErr
	})
	return response, err
}

func (s *Service) invokeExecutorProvider(
	ctx context.Context,
	requestContext *pluginsdk.ExecutorProviderRequestContext,
	call func(context.Context, *pluginsdk.RemotePlugin, *pluginsdk.ExecutorProviderRequestContext) error,
) error {
	if requestContext == nil || requestContext.GetPluginId() == "" || requestContext.GetProviderKey() == "" || call == nil {
		return fmt.Errorf("%w: provider operation identity is incomplete", ErrExecutorProviderUnavailable)
	}
	return s.withExecutorProvider(ctx, requestContext.GetPluginId(), requestContext.GetProviderKey(), func(callCtx context.Context, remote *pluginsdk.RemotePlugin, declaration *manifest.ExecutorProvider) error {
		record, err := s.Get(requestContext.GetPluginId())
		if err != nil || record == nil || requestContext.GetInstallationId() != record.InstallationID || requestContext.GetContractVersion() != int32(declaration.ContractVersion) {
			return fmt.Errorf("%w: provider operation identity is stale", ErrExecutorProviderUnavailable)
		}
		trusted := proto.Clone(requestContext).(*pluginsdk.ExecutorProviderRequestContext)
		trusted.PluginId = record.ID
		trusted.InstallationId = record.InstallationID
		trusted.ProviderKey = declaration.Key
		trusted.ContractVersion = int32(declaration.ContractVersion)
		activeContext, release := s.beginExecutorProviderOperation(trusted)
		defer release()
		return call(callCtx, remote, activeContext)
	})
}

func validateExecutorProviderAdmission(enabled bool, record *store.Record, key string) (*manifest.ExecutorProvider, error) {
	if !enabled {
		return nil, ErrExecutorProvidersDisabled
	}
	if record == nil || record.Status != StatusActive || !record.Capabilities.ExecutorProvider {
		return nil, ErrExecutorProviderUnavailable
	}
	for index := range record.ExecutorProviders {
		provider := &record.ExecutorProviders[index]
		if provider.Key != key {
			continue
		}
		if provider.ContractVersion != manifest.CurrentExecutorProviderContractVersion {
			return nil, fmt.Errorf("%w: unsupported contract version", ErrExecutorProviderUnavailable)
		}
		return provider, nil
	}
	return nil, fmt.Errorf("%w: provider is not declared", ErrExecutorProviderUnavailable)
}

func validateExecutorProviderContracts(ctx context.Context, record *store.Record, probe executorProviderContractProbe) error {
	if record == nil || len(record.ExecutorProviders) == 0 {
		return nil
	}
	if !record.Capabilities.ExecutorProvider || probe == nil {
		return fmt.Errorf("%w: provider contract is unavailable", ErrExecutorProviderUnavailable)
	}
	for _, provider := range record.ExecutorProviders {
		response, err := probe.ValidateExecutorProfile(ctx, &pluginsdk.ValidateExecutorProfileRequest{
			Context: &pluginsdk.ExecutorProviderRequestContext{
				PluginId: record.ID, InstallationId: record.InstallationID, ProviderKey: provider.Key,
				ContractVersion: int32(provider.ContractVersion),
			},
			Profile: &pluginsdk.ExecutorProfileSnapshot{Config: map[string]string{}},
		})
		if err != nil {
			return fmt.Errorf("%w: plugin does not implement the complete executor provider contract", ErrExecutorProviderUnavailable)
		}
		if response == nil {
			return fmt.Errorf("%w: plugin returned an empty executor provider response", ErrExecutorProviderUnavailable)
		}
	}
	return nil
}

// withExecutorProvider holds a plugin dispatch read lease through the RPC.
// Lifecycle changes close admission and cancel active call contexts before
// taking the corresponding write lease to drain calls and stop the process.
func (s *Service) withExecutorProvider(
	ctx context.Context,
	pluginID string,
	providerKey string,
	call func(context.Context, *pluginsdk.RemotePlugin, *manifest.ExecutorProvider) error,
) error {
	if call == nil {
		return errors.New("plugins: executor provider callback is required")
	}
	enabled := s.remoteExecutorProvidersEnabled()
	if !enabled {
		return ErrExecutorProvidersDisabled
	}

	lock := s.dispatchLocks.lockFor(pluginID)
	lock.RLock()
	defer lock.RUnlock()
	record, err := s.Get(pluginID)
	if err != nil {
		return err
	}
	provider, err := validateExecutorProviderAdmission(enabled, record, providerKey)
	if err != nil {
		return err
	}
	remote, ok := s.pluginRemote(pluginID)
	if !ok || remote == nil {
		return fmt.Errorf("%w: plugin process is not running", ErrExecutorProviderUnavailable)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	callCtx, release, err := s.beginExecutorProviderDispatch(ctx, pluginID)
	if err != nil {
		return err
	}
	defer release()
	return call(callCtx, remote, provider)
}

func (s *Service) remoteExecutorProvidersEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.remoteExecutorPluginsEnabled
}

func (s *Service) validateExecutorProviderRuntime(ctx context.Context, record *store.Record) error {
	if !s.remoteExecutorProvidersEnabled() || record == nil || len(record.ExecutorProviders) == 0 {
		return nil
	}
	remote, ok := s.pluginRemote(record.ID)
	if !ok || remote == nil {
		return fmt.Errorf("%w: plugin process is not running", ErrExecutorProviderUnavailable)
	}
	return validateExecutorProviderContracts(ctx, record, remote)
}
