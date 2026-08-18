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

func (s *Service) warnLoadedLegacyWebhookAccess() {
	for _, record := range s.List() {
		s.warnUndeclaredLegacyWebhookAccess(record.Manifest)
	}
}
