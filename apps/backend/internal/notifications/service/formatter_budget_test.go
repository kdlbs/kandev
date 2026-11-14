package service

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kandev/kandev/internal/notifications/providers"
	userstore "github.com/kandev/kandev/internal/user/store"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type timeoutFormatterRuntime struct {
	formatterRuntimeFixture
	calls int
}

func (f *timeoutFormatterRuntime) FormatNotification(ctx context.Context, _ *models.FormatterBinding, _ *pluginsdk.NotificationFormatRequest) (*pluginsdk.NotificationFormatResponse, error) {
	f.calls++
	<-ctx.Done()
	synctest.Wait()
	return nil, ctx.Err()
}

func TestFormatterFanoutSharesBudgetWithoutCancellingDelivery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log, err := logger.NewFromZap(zap.NewNop())
		require.NoError(t, err)
		repo := &formatterRepositoryFixture{notificationTestRepository: &notificationTestRepository{
			providers: []*models.Provider{
				{ID: "first", UserID: userstore.DefaultUserID, Type: models.ProviderTypeApprise, Enabled: true},
				{ID: "second", UserID: userstore.DefaultUserID, Type: models.ProviderTypeApprise, Enabled: true},
			}, subscriptions: map[string][]*models.Subscription{
				"first":  {{EventType: EventTaskSessionClarificationAsked, Enabled: true}},
				"second": {{EventType: EventTaskSessionClarificationAsked, Enabled: true}},
			}}, bindings: map[string]*models.FormatterBinding{
			"first":  {ProviderID: "first", ProviderType: models.ProviderTypeApprise, PluginID: "formatter"},
			"second": {ProviderID: "second", ProviderType: models.ProviderTypeApprise, PluginID: "formatter"},
		}}
		svc := NewService(repo, nil, nil, log, nil, false)
		adapter := &captureProvider{}
		svc.providers = map[models.ProviderType]providers.Provider{models.ProviderTypeApprise: adapter}
		runtime := &timeoutFormatterRuntime{}
		svc.SetFormatterRuntime(runtime)
		ctx, cancel := context.WithTimeout(t.Context(), 1500*time.Millisecond)
		defer cancel()
		svc.deliverOccurrence(ctx, userstore.DefaultUserID, notificationPayload{OccurrenceID: "question", EventType: EventTaskSessionClarificationAsked, Title: "Default title", Body: "Default body"})
		require.NoError(t, ctx.Err(), "formatter delays must not exhaust the caller's delivery budget")
		require.Equal(t, 1, runtime.calls)
		require.Len(t, adapter.messages, 2)
		for _, message := range adapter.messages {
			require.Equal(t, "Default title", message.Title)
			require.Equal(t, "Default body", message.Body)
		}
	})
}
