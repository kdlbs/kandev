package config

import (
	commonconfig "github.com/kandev/kandev/internal/common/config"
	"github.com/stretchr/testify/require"
	"reflect"
	"testing"
	"time"
)

// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-002.5
func TestContinuationConfigManagedStartup(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		startup := commonconfig.AgentctlStartupConfig{Configured: true, IdleReaperInterval: time.Minute, NotificationQueueCapacity: 4096, ProviderInterruptionContinuation: enabled}
		cfg, err := LoadWithStartup(startup)
		require.NoError(t, err)
		field := reflect.ValueOf(cfg).Elem().FieldByName("ProviderInterruptionContinuation")
		require.True(t, field.IsValid(), "server must carry managed continuation flag")
		require.Equal(t, enabled, field.Bool())
		instance := reflect.ValueOf(cfg.NewInstanceConfig(41001, nil)).Elem().FieldByName("ProviderInterruptionContinuation")
		require.True(t, instance.IsValid(), "instance must inherit continuation flag")
		require.Equal(t, enabled, instance.Bool())
	}
}
