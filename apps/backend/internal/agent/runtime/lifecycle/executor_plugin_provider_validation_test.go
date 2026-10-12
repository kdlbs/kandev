package lifecycle

import (
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func TestPluginExecutorResourceStateMatchesProviderSchema(t *testing.T) {
	provider := testPluginExecutorLaunchProvider()
	provider.ResourceStateSchema = map[string]any{
		"type": "object", "required": []any{"region"}, "additionalProperties": false,
		"properties": map[string]any{
			"region":  map[string]any{"type": "string", "enum": []any{"eu-west-1", "us-east-1"}},
			"workers": map[string]any{"type": "integer", "minimum": float64(1), "maximum": float64(8)},
		},
	}
	tests := []struct {
		name      string
		state     string
		wantError bool
	}{
		{name: "valid", state: `{"region":"eu-west-1","workers":2}`},
		{name: "missing required", state: `{"workers":2}`, wantError: true},
		{name: "wrong type", state: `{"region":true}`, wantError: true},
		{name: "outside enum", state: `{"region":"ap-south-1"}`, wantError: true},
		{name: "outside numeric bounds", state: `{"region":"eu-west-1","workers":9}`, wantError: true},
		{name: "undeclared credential field", state: `{"region":"eu-west-1","api_token":"credential"}`, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resource := &pluginsdk.ExecutorResourceDescriptor{
				ResourceHandle: "resource-1", StateVersion: 1, StateJson: test.state,
			}
			err := validatePluginExecutorResourceForProvider(provider, resource)
			if (err != nil) != test.wantError {
				t.Fatalf("validatePluginExecutorResourceForProvider() error = %v, wantError %v", err, test.wantError)
			}
		})
	}
}

func TestPluginExecutorBoundedResourceRequiresValidExpiry(t *testing.T) {
	provider := testPluginExecutorLaunchProvider()
	provider.Capabilities = models.ExecutorProviderCapabilities{Retention: "bounded", MaximumLifetimeSecs: 300}
	validExpiry := time.Now().UTC().Add(2 * time.Minute).Format(time.RFC3339Nano)
	tests := []struct {
		name      string
		expiresAt string
		wantError bool
	}{
		{name: "valid", expiresAt: validExpiry},
		{name: "missing", wantError: true},
		{name: "malformed", expiresAt: "tomorrow", wantError: true},
		{name: "beyond maximum lifetime", expiresAt: time.Now().UTC().Add(6 * time.Minute).Format(time.RFC3339Nano), wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resource := &pluginsdk.ExecutorResourceDescriptor{
				ResourceHandle: "resource-1", StateVersion: 1, StateJson: `{}`, Retention: "bounded", ExpiresAt: test.expiresAt,
				Capabilities: &pluginsdk.ExecutorProviderCapabilities{Retention: "bounded"},
			}
			err := validatePluginExecutorResourceForProvider(provider, resource)
			if (err != nil) != test.wantError {
				t.Fatalf("validatePluginExecutorResourceForProvider() error = %v, wantError %v", err, test.wantError)
			}
			if err != nil && !test.wantError {
				t.Fatalf("valid bounded resource rejected: %v", err)
			}
		})
	}
}

func TestPluginExecutorResourceExpiryRejectsInvalidAbsoluteDate(t *testing.T) {
	provider := testPluginExecutorLaunchProvider()
	provider.Capabilities = models.ExecutorProviderCapabilities{Retention: "bounded", MaximumLifetimeSecs: 900}
	resource := &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: "resource-1", StateVersion: 1, StateJson: `{}`, Retention: "bounded",
		Capabilities: &pluginsdk.ExecutorProviderCapabilities{Retention: "bounded"},
	}
	for _, value := range []string{"", "2026-09-27", "2026-09-27T00:00:00"} {
		t.Run(fmt.Sprintf("expires_at_%q", value), func(t *testing.T) {
			resource.ExpiresAt = value
			if err := validatePluginExecutorResourceForProvider(provider, resource); err == nil {
				t.Fatalf("expiry %q passed bounded resource validation", value)
			}
		})
	}
}
