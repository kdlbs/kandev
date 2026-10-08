package config

import (
	"encoding/json"
	"reflect"
	"testing"

	commonconfig "github.com/kandev/kandev/internal/common/config"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-002.5
func TestLegacyContinuationStartupSettingIsIgnored(t *testing.T) {
	t.Setenv("KANDEV_FEATURES_PROVIDER_INTERRUPTION_CONTINUATION", "false")
	startup, err := commonconfig.DecodeAgentctlStartupConfig(`{"configured":true,"idleReaperInterval":60000000000,"notificationQueueCapacity":4096,"providerInterruptionContinuation":false}`)
	require.NoError(t, err)

	encodedStartup, err := json.Marshal(startup)
	require.NoError(t, err)
	require.NotContains(t, string(encodedStartup), "providerInterruptionContinuation")

	managed, err := LoadWithStartup(startup)
	require.NoError(t, err)
	standalone := Load()
	instance := managed.NewInstanceConfig(41001, nil)
	for _, typ := range []reflect.Type{reflect.TypeOf(startup), reflect.TypeOf(managed).Elem(), reflect.TypeOf(standalone).Elem(), reflect.TypeOf(instance).Elem()} {
		_, exists := typ.FieldByName("ProviderInterruptionContinuation")
		require.False(t, exists, "%s must not carry the retired setting", typ)
	}
}
