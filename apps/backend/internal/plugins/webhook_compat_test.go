package plugins

import (
	"reflect"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/plugins/manifest"
)

func TestUndeclaredLegacyWebhookKeys(t *testing.T) {
	tests := []struct {
		name     string
		manifest manifest.Manifest
		want     []string
	}{
		{
			name: "v1 omitted access uses compatibility default",
			manifest: manifest.Manifest{APIVersion: manifest.LegacyAPIVersion, Webhooks: []manifest.Webhook{
				{Key: "events"},
				{Key: "callback", Access: manifest.WebhookAccessPublic},
				{Key: "upload", Access: manifest.WebhookAccessAuthenticated},
				{Key: "commands"},
			}},
			want: []string{"events", "commands"},
		},
		{
			name: "v2 omission is intentional authenticated default",
			manifest: manifest.Manifest{APIVersion: manifest.CurrentAPIVersion, Webhooks: []manifest.Webhook{
				{Key: "events"},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := undeclaredLegacyWebhookKeys(tt.manifest); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("undeclaredLegacyWebhookKeys() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLegacyWebhookWarningNamesPluginAndKeys(t *testing.T) {
	core, observed := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	svc := &Service{log: log}
	svc.warnUndeclaredLegacyWebhookAccess(manifest.Manifest{
		ID:         "kandev-plugin-legacy",
		APIVersion: manifest.LegacyAPIVersion,
		Webhooks:   []manifest.Webhook{{Key: "events"}, {Key: "commands"}},
	})

	entries := observed.All()
	if len(entries) != 1 {
		t.Fatalf("warning count = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["plugin_id"] != "kandev-plugin-legacy" ||
		!reflect.DeepEqual(fields["webhook_keys"], []any{"events", "commands"}) {
		t.Fatalf("warning fields = %#v, want plugin id and both webhook keys", fields)
	}
}
