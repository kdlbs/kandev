package backendapp

import (
	agentsettingscatalog "github.com/kandev/kandev/internal/agent/settings/catalog"
	"github.com/kandev/kandev/internal/settingscatalog"
)

// buildSettingsRegistry composes the platform catalog with the agent-settings
// catalog so runtime validation and generated discovery use one profile
// contract.
func buildSettingsRegistry() (*settingscatalog.Registry, error) {
	profileDomains := agentsettingscatalog.Descriptors()
	profileTypes := make(map[string]struct{}, len(profileDomains))
	for _, domain := range profileDomains {
		profileTypes[domain.ResourceType] = struct{}{}
	}

	domains := make([]settingscatalog.DomainDescriptor, 0)
	for _, domain := range settingscatalog.DefaultDomainDescriptors() {
		if _, replaced := profileTypes[domain.ResourceType]; replaced {
			continue
		}
		domains = append(domains, domain)
	}
	domains = append(domains, profileDomains...)
	return settingscatalog.NewRegistry(domains)
}
