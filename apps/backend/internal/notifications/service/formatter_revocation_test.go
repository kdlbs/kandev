package service

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kandev/kandev/internal/notifications/providers"
	userstore "github.com/kandev/kandev/internal/user/store"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type revokingFormatterRuntime struct {
	formatterRuntimeFixture
	revoke          func()
	waitForDeadline bool
}

func (f *revokingFormatterRuntime) FormatNotification(ctx context.Context, _ *models.FormatterBinding, _ *pluginsdk.NotificationFormatRequest) (*pluginsdk.NotificationFormatResponse, error) {
	f.revoke()
	if f.waitForDeadline {
		<-ctx.Done()
		synctest.Wait()
	}
	return &pluginsdk.NotificationFormatResponse{Custom: true, Title: "Custom subject", Body: "Custom body"}, nil
}

func TestFormatterFanoutRejectsRevocationAfterBindingCaptureBecomesUnavailable(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "live-budget", true: "expired-budget"}[expired], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				log, err := logger.NewFromZap(zap.NewNop())
				require.NoError(t, err)
				first := &models.Provider{ID: "first", UserID: userstore.DefaultUserID, Type: models.ProviderTypeApprise, Enabled: true}
				second := &models.Provider{ID: "second", UserID: userstore.DefaultUserID, Type: models.ProviderTypeApprise, Enabled: true}
				repo := &formatterRepositoryFixture{
					notificationTestRepository: &notificationTestRepository{
						providers: []*models.Provider{first, second},
						subscriptions: map[string][]*models.Subscription{
							"first":  {{EventType: EventTaskSessionTurnFinished, Enabled: true}},
							"second": {{EventType: EventTaskSessionTurnFinished, Enabled: true}},
						},
					},
					bindings: map[string]*models.FormatterBinding{
						"first":  {ProviderID: "first", ProviderType: models.ProviderTypeApprise, PluginID: "formatter"},
						"second": {ProviderID: "second", ProviderType: models.ProviderTypeApprise, PluginID: "formatter"},
					},
				}
				svc := NewService(repo, nil, nil, log, nil, false)
				capture := &captureProvider{}
				svc.providers = map[models.ProviderType]providers.Provider{models.ProviderTypeApprise: capture}
				svc.SetFormatterRuntime(&revokingFormatterRuntime{waitForDeadline: expired, revoke: func() {
					repo.mu.Lock()
					defer repo.mu.Unlock()
					repo.providers = []*models.Provider{first}
					delete(repo.bindings, second.ID)
				}})
				svc.HandleTaskTurnFinished(t.Context(), "task", "session", "turn")
				require.Len(t, capture.messages, 1, "a later revoked provider must not bypass the fence through fallback")
			})
		})
	}
}

func TestFormatterDeliveryRejectsRevokedProviderAfterRPC(t *testing.T) {
	t.Setenv(desktopNativeNotificationsEnv, "true")
	for _, change := range []string{"unchanged", "deleted", "type", "owner", "disabled", "unsubscribed"} {
		t.Run(change, func(t *testing.T) {
			log, err := logger.NewFromZap(zap.NewNop())
			require.NoError(t, err)
			provider := &models.Provider{ID: "mail", UserID: userstore.DefaultUserID, Type: models.ProviderTypeApprise, Enabled: true}
			repo := &formatterRepositoryFixture{
				notificationTestRepository: &notificationTestRepository{
					providers: []*models.Provider{provider},
					subscriptions: map[string][]*models.Subscription{
						"mail": {{EventType: EventTaskSessionTurnFinished, Enabled: true}},
					},
				},
				binding: &models.FormatterBinding{ProviderID: "mail", ProviderType: models.ProviderTypeApprise, PluginID: "formatter"},
			}
			svc := NewService(repo, nil, nil, log, nil, false)
			capture := &captureProvider{}
			svc.providers = map[models.ProviderType]providers.Provider{models.ProviderTypeApprise: capture}
			svc.SetFormatterRuntime(&revokingFormatterRuntime{revoke: func() {
				repo.mu.Lock()
				defer repo.mu.Unlock()
				current := *provider
				switch change {
				case "deleted":
					repo.providers = nil
					return
				case "type":
					current.Type = models.ProviderTypeLocal
				case "owner":
					current.UserID = "another-owner"
				case "disabled":
					current.Enabled = false
				case "unsubscribed":
					repo.subscriptions["mail"] = nil
				}
				repo.providers = []*models.Provider{&current}
			}})
			svc.HandleTaskTurnFinished(t.Context(), "task", "session", "turn")
			if change == "unchanged" {
				require.Len(t, capture.messages, 1)
				require.Equal(t, "Custom subject", capture.messages[0].Title)
			} else {
				require.Empty(t, capture.messages, "revoked provider must not receive custom or fallback content")
			}
		})
	}
}
