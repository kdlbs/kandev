package models

type FormatterDescriptor struct {
	PluginID       string `json:"plugin_id"`
	InstallationID string `json:"installation_id"`
	Key            string `json:"key"`
	Version        string `json:"version"`
}

// FormatterBinding selects one immutable plugin configuration for a provider.
// An empty PluginID is a retained clear revision, not an absent revision.
type FormatterBinding struct {
	ProviderID            string       `json:"provider_id" db:"provider_id"`
	ProviderType          ProviderType `json:"provider_type" db:"provider_type"`
	PluginID              string       `json:"plugin_id" db:"plugin_id"`
	InstallationID        string       `json:"installation_id" db:"installation_id"`
	FormatterKey          string       `json:"formatter_key" db:"formatter_key"`
	ConfigurationRevision string       `json:"configuration_revision" db:"configuration_revision"`
	Digest                string       `json:"digest" db:"digest"`
	Revision              string       `json:"revision" db:"revision"`
}
