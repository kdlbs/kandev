package manifest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNotificationFormatterDeclarations(t *testing.T) {
	m := validManifest(t)
	m.BaseURL = ""
	m.Endpoints = Endpoints{}
	m.Runtime = Runtime{Type: "binary", Executables: map[string]string{"darwin-arm64": "server/plugin"}}
	m.Actions = []Action{{Key: "templates.prepare", ResourceScope: ActionScopeNotificationProvider, MaxBodyBytes: 131072}}
	m.NotificationFormatters = []NotificationFormatter{{Key: "text", ContractVersion: 1, ProviderTypes: []string{"apprise"}, ManagementActions: []string{"templates.prepare"}}}
	require.NoError(t, m.Validate())
	m.NotificationFormatters[0].ManagementActions = []string{"undeclared"}
	require.ErrorContains(t, m.Validate(), "undeclared")
	m.NotificationFormatters[0].ManagementActions = []string{"templates.prepare"}
	m.NotificationFormatters[0].ContractVersion = 99
	require.Error(t, m.Validate())
}
