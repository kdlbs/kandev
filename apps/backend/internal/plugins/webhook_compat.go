package plugins

import (
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/plugins/manifest"
)

func undeclaredLegacyWebhookKeys(m manifest.Manifest) []string {
	if m.APIVersion != manifest.LegacyAPIVersion {
		return nil
	}
	keys := make([]string, 0, len(m.Webhooks))
	for _, webhook := range m.Webhooks {
		if webhook.Access == "" {
			keys = append(keys, webhook.Key)
		}
	}
	return keys
}

func (s *Service) warnUndeclaredLegacyWebhookAccess(m manifest.Manifest) {
	keys := undeclaredLegacyWebhookKeys(m)
	if len(keys) == 0 {
		return
	}
	s.log.Warn(
		"plugins: api_version 1 uses the deprecated public webhook access default",
		zap.String("plugin_id", m.ID),
		zap.Strings("webhook_keys", keys),
		zap.String("migration", "set access explicitly or upgrade the manifest to api_version 2"),
	)
}

func nonPublicSSOInitiateProviderIDs(m manifest.Manifest) []string {
	if !m.Capabilities.Auth {
		return nil
	}
	accessByKey := make(map[string]string, len(m.Webhooks))
	for _, webhook := range m.Webhooks {
		accessByKey[webhook.Key] = webhook.EffectiveAccess(m.APIVersion)
	}
	var providerIDs []string
	for _, provider := range m.AuthProviders {
		access, declared := accessByKey[provider.Initiate]
		if provider.ID != "" && declared && access != manifest.WebhookAccessPublic {
			providerIDs = append(providerIDs, provider.ID)
		}
	}
	return providerIDs
}

func (s *Service) warnNonPublicSSOInitiates(m manifest.Manifest) {
	providerIDs := nonPublicSSOInitiateProviderIDs(m)
	if len(providerIDs) == 0 {
		return
	}
	s.log.Warn(
		"plugins: SSO providers require a public initiate webhook",
		zap.String("plugin_id", m.ID),
		zap.Strings("auth_provider_ids", providerIDs),
		zap.String("migration", "set access: public on each provider's initiate webhook"),
	)
}

func (s *Service) warnWebhookAccessIssues(m manifest.Manifest) {
	s.warnUndeclaredLegacyWebhookAccess(m)
	s.warnNonPublicSSOInitiates(m)
}

func (s *Service) warnLoadedWebhookAccessIssues() {
	for _, record := range s.List() {
		s.warnWebhookAccessIssues(record.Manifest)
	}
}
