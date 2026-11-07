package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kandev/kandev/internal/notifications/providers"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	userstore "github.com/kandev/kandev/internal/user/store"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type formatterRepositoryFixture struct {
	*notificationTestRepository
	binding *models.FormatterBinding
}

func TestFormatterPreviewUsesSyntheticContextWithoutDelivery(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	repo := &notificationTestRepository{providers: []*models.Provider{{ID: "mail", UserID: "alice", Type: models.ProviderTypeApprise}}}
	svc := NewService(repo, nil, nil, log, nil, false)
	runtime := &formatterRuntimeFixture{result: &pluginsdk.NotificationFormatResponse{Custom: true, Title: "Preview", Body: "Synthetic"}}
	svc.SetFormatterRuntime(runtime)
	result, err := svc.PreviewFormatter(context.Background(), "alice", "mail", models.FormatterBinding{PluginID: "formatter", InstallationID: "install", FormatterKey: "text"}, EventTaskSessionFailed, "normal", []byte(`{"version":1}`))
	require.NoError(t, err)
	require.Equal(t, "Preview", result.Title)
	require.True(t, runtime.request.Preview)
	require.Equal(t, "example-task", runtime.request.Task.TaskId)
	require.Empty(t, repo.deliveries)
}

func (r *formatterRepositoryFixture) GetFormatterBinding(context.Context, string, string) (*models.FormatterBinding, error) {
	return r.binding, nil
}

func (r *formatterRepositoryFixture) SetFormatterBinding(context.Context, string, string, string, *models.FormatterBinding) (*models.FormatterBinding, error) {
	panic("delivery must not mutate bindings")
}

type formatterRuntimeFixture struct {
	request *pluginsdk.NotificationFormatRequest
	result  *pluginsdk.NotificationFormatResponse
	err     error
}

func (f *formatterRuntimeFixture) NotificationFormatters(context.Context, models.ProviderType) ([]models.FormatterDescriptor, error) {
	return nil, nil
}

func (f *formatterRuntimeFixture) LeaseFormatterConfiguration(context.Context, string, *models.FormatterBinding) (*pluginsdk.NotificationConfigurationResponse, func(), error) {
	panic("delivery must use the captured revision")
}

func (f *formatterRuntimeFixture) FormatNotification(_ context.Context, _ *models.FormatterBinding, req *pluginsdk.NotificationFormatRequest) (*pluginsdk.NotificationFormatResponse, error) {
	f.request = req
	return f.result, f.err
}

func TestFormatterDeliveryRetainsAdmissionAndWholePairFallback(t *testing.T) {
	t.Setenv(desktopNativeNotificationsEnv, "true")
	for _, invalid := range []bool{false, true} {
		t.Run(map[bool]string{false: "custom", true: "fallback"}[invalid], func(t *testing.T) {
			log, err := logger.NewFromZap(zap.NewNop())
			require.NoError(t, err)
			repo := &formatterRepositoryFixture{
				notificationTestRepository: &notificationTestRepository{
					providers:     []*models.Provider{{ID: "mail", UserID: userstore.DefaultUserID, Type: models.ProviderTypeApprise, Enabled: true}},
					subscriptions: map[string][]*models.Subscription{"mail": {{EventType: EventTaskSessionTurnFinished, Enabled: true}}},
				},
				binding: &models.FormatterBinding{ProviderID: "mail", PluginID: "formatter", ConfigurationRevision: "snapshot-a"},
			}
			svc := NewService(repo, notificationTestTaskGetter{task: &taskmodels.Task{ID: "task", Title: "Example"}}, nil, log, nil, false)
			capture := &captureProvider{}
			svc.providers = map[models.ProviderType]providers.Provider{models.ProviderTypeApprise: capture}
			runtime := &formatterRuntimeFixture{result: &pluginsdk.NotificationFormatResponse{Custom: true, Title: "Custom subject", Body: "Custom body"}}
			if invalid {
				runtime.result.Title = "bad\r\nheader"
			}
			svc.SetFormatterRuntime(runtime)
			svc.HandleTaskTurnFinished(context.Background(), "task", "session", "turn")
			svc.HandleTaskTurnFinished(context.Background(), "task", "session", "turn")
			require.Len(t, capture.messages, 1)
			require.Equal(t, "snapshot-a", runtime.request.ConfigurationRevision)
			require.Empty(t, runtime.request.Task.SessionId, "an unverified session must not become a template identity")
			if invalid {
				require.Equal(t, "Agent turn finished", capture.messages[0].Title)
				require.Equal(t, "The agent finished a turn on \"Example\".", capture.messages[0].Body)
			} else {
				require.Equal(t, "Custom subject", capture.messages[0].Title)
				require.Equal(t, "Custom body", capture.messages[0].Body)
			}
		})
	}
}
