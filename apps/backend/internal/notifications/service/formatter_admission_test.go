package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kandev/kandev/internal/notifications/providers"
	userstore "github.com/kandev/kandev/internal/user/store"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type formatterAdmissionRepository struct{ formatterRepositoryFixture }

func (r *formatterAdmissionRepository) InsertDelivery(ctx context.Context, delivery *models.Delivery) (bool, error) {
	r.binding.ConfigurationRevision = "snapshot-b"
	return r.notificationTestRepository.InsertDelivery(ctx, delivery)
}

func TestRuntimeSummaryRetainsFormatterCapturedBeforeDeliveryClaims(t *testing.T) {
	t.Setenv(desktopNativeNotificationsEnv, "true")
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	repository := &formatterAdmissionRepository{formatterRepositoryFixture: formatterRepositoryFixture{
		notificationTestRepository: &notificationTestRepository{
			providers:     []*models.Provider{{ID: "mail", UserID: userstore.DefaultUserID, Type: models.ProviderTypeApprise, Enabled: true}},
			subscriptions: map[string][]*models.Subscription{"mail": {{EventType: EventSystemUpdateAvailable, Enabled: true}}},
		}, binding: &models.FormatterBinding{ProviderID: "mail", PluginID: "formatter", ConfigurationRevision: "snapshot-a"},
	}}
	service := NewService(repository, nil, nil, log, nil, false)
	runtime := &formatterRuntimeFixture{result: &pluginsdk.NotificationFormatResponse{}}
	service.SetFormatterRuntime(runtime)
	capture := &captureProvider{}
	service.providers = map[models.ProviderType]providers.Provider{models.ProviderTypeApprise: capture}
	service.HandleAgentRuntimeUpdates(context.Background(), []agents.RuntimeUpdateNotice{
		{AgentID: "one", RuntimeID: "runtime-one", Version: "1.2.3", OccurrenceID: "first", Status: "available"},
		{AgentID: "two", RuntimeID: "runtime-two", Version: "1.2.3", OccurrenceID: "second", Status: "available"},
	})
	require.NotNil(t, runtime.request)
	require.Equal(t, "snapshot-a", runtime.request.ConfigurationRevision)
	require.Len(t, capture.messages, 1)
}
