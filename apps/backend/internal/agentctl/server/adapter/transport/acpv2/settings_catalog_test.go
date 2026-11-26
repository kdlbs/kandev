package acpv2

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func TestWireModelOptionCatalog(t *testing.T) {
	fixture := newWireFixture(t)
	fixture.initialize()
	done := asyncCall(func() error {
		_, err := fixture.adapter.NewSession(fixture.ctx, nil)
		return err
	})
	request := fixture.read("session/new")
	fixture.reply(request, map[string]any{"sessionId": fixtureSession, "configOptions": modelCatalogOptions("model-a", true)})
	require.NoError(t, fixture.wait(done))
	fixture.event(streams.EventTypeSessionStatus)
	setup := fixture.event(streams.EventTypeSessionModels)
	require.Equal(t, true, setup.Data[streams.ConfigOptionsCompleteKey])
	require.True(t, fixture.adapter.GetSessionModelState().ConfigOptionsComplete)

	var result streams.ConfigurationResult
	done = asyncCall(func() error {
		var err error
		result, err = fixture.adapter.SetModelWithResult(fixture.ctx, "model-b")
		return err
	})
	request = fixture.read("session/set_config_option")
	require.Equal(t, "model-b", request.Params["value"])
	fixture.reply(request, map[string]any{"configOptions": modelCatalogOptions("model-b", false)})
	require.NoError(t, fixture.wait(done))
	require.True(t, result.Confirmed)
	require.NotNil(t, result.ModelState)
	changed := fixture.event(streams.EventTypeSessionModels)
	require.Equal(t, true, changed.Data[streams.ConfigOptionsCompleteKey])
	require.Equal(t, changed.SessionSettingsGeneration, result.ModelState.SessionSettingsGeneration)
	require.Greater(t, changed.SessionSettingsGeneration, setup.SessionSettingsGeneration)
	require.Equal(t, "model-b", result.ModelState.CurrentModelID)
	require.Equal(t, fixtureSession, result.ModelState.SessionID)
	require.Len(t, result.ModelState.ConfigOptions, 1)
	require.Len(t, setup.ConfigOptions, 2)
	require.Equal(t, "false", setup.ConfigOptions[1].CurrentValue)

	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "config_option_update", "configOptions": modelCatalogOptions("model-a", true),
	})
	updated := fixture.event(streams.EventTypeSessionModels)
	require.Equal(t, true, updated.Data[streams.ConfigOptionsCompleteKey])
	require.Equal(t, streams.ConfigOptionsSourceProviderUpdate, updated.Data[streams.ConfigOptionsSourceKey])
	require.Equal(t, "model-a", updated.CurrentModelID)
	require.Greater(t, updated.SessionSettingsGeneration, changed.SessionSettingsGeneration)
}

func modelCatalogOptions(model string, includeExtra bool) []map[string]any {
	options := []map[string]any{{
		"configId": "provider-model", "name": "Model", "category": "model", "type": "select", "currentValue": model,
		"options": []map[string]any{{"value": "model-a", "name": "Model A"}, {"value": "model-b", "name": "Model B"}},
	}}
	if includeExtra {
		options = append(options, map[string]any{
			"configId": "extra", "name": "Extra", "type": "boolean", "currentValue": false,
		})
	}
	return options
}
