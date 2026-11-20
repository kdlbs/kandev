package lifecycle

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
)

func TestOpenCodePluginRuntimeEligibility(t *testing.T) {
	openCode := agents.NewOpenCodeACP()
	claude := agents.NewClaudeACP()
	cases := []struct {
		name    string
		agent   agents.Agent
		options agents.CommandOptions
		enabled bool
		want    bool
	}{
		{
			name:  "supported managed v2",
			agent: openCode, enabled: true,
			options: agents.CommandOptions{
				ManagedRuntimeFamily:  managedruntime.OpenCodeFamilyV2,
				ManagedRuntimeSource:  managedruntime.OpenCodeSourceManaged,
				ManagedRuntimeVersion: "2.2.0",
			},
			want: true,
		},
		{
			name:  "supported native v2 uses the detected version",
			agent: openCode, enabled: true,
			options: agents.CommandOptions{
				ManagedRuntimeFamily:  managedruntime.OpenCodeFamilyV2,
				ManagedRuntimeSource:  managedruntime.OpenCodeSourceNative,
				ManagedRuntimeVersion: "2.1.0",
				NativeRuntimeVersion:  "2.2.0",
			},
			want: true,
		},
		{
			name:  "unsupported v2 version",
			agent: openCode, enabled: true,
			options: agents.CommandOptions{
				ManagedRuntimeFamily:  managedruntime.OpenCodeFamilyV2,
				ManagedRuntimeSource:  managedruntime.OpenCodeSourceManaged,
				ManagedRuntimeVersion: "2.2.1",
			},
		},
		{
			name:  "v1 stays legacy",
			agent: openCode, enabled: true,
			options: agents.CommandOptions{
				ManagedRuntimeFamily:  managedruntime.OpenCodeFamilyV1,
				ManagedRuntimeSource:  managedruntime.OpenCodeSourceManaged,
				ManagedRuntimeVersion: "1.19.4",
			},
		},
		{
			name:  "flag disabled",
			agent: openCode, enabled: false,
			options: agents.CommandOptions{
				ManagedRuntimeFamily:  managedruntime.OpenCodeFamilyV2,
				ManagedRuntimeSource:  managedruntime.OpenCodeSourceManaged,
				ManagedRuntimeVersion: "2.2.0",
			},
		},
		{
			name:  "other provider",
			agent: claude, enabled: true,
			options: agents.CommandOptions{
				ManagedRuntimeFamily:  managedruntime.OpenCodeFamilyV2,
				ManagedRuntimeSource:  managedruntime.OpenCodeSourceManaged,
				ManagedRuntimeVersion: "2.2.0",
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			evidence := openCodePluginRuntimeFor(testCase.agent, testCase.options, testCase.enabled)
			if !testCase.want {
				require.Nil(t, evidence)
				return
			}
			require.NotNil(t, evidence)
			require.Equal(t, "v2", evidence.Family)
			require.Equal(t, "2.2.0", evidence.Version)
		})
	}
}
