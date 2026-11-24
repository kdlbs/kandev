package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type activationRuntimeFixture struct {
	formatterRuntimeFixture
	held bool
}

func (f *activationRuntimeFixture) LeaseFormatterConfiguration(context.Context, string, *models.FormatterBinding) (*pluginsdk.NotificationConfigurationResponse, func(), error) {
	f.held = true
	return &pluginsdk.NotificationConfigurationResponse{Revision: "snapshot", Digest: "digest"}, func() { f.held = false }, nil
}

type activationRepositoryFixture struct {
	formatterRepositoryFixture
	beforeCommit func()
}

func (r *activationRepositoryFixture) SetFormatterBinding(_ context.Context, _, _, _ string, binding *models.FormatterBinding) (*models.FormatterBinding, error) {
	r.beforeCommit()
	return binding, nil
}

func TestFormatterActivationRetainsInstallationLeaseThroughBindingCommit(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	runtime := &activationRuntimeFixture{}
	repository := &activationRepositoryFixture{formatterRepositoryFixture: formatterRepositoryFixture{notificationTestRepository: &notificationTestRepository{
		providers: []*models.Provider{{ID: "mail", UserID: "alice", Type: models.ProviderTypeApprise}},
	}}, beforeCommit: func() {
		require.True(t, runtime.held, "installation may not change between inspection and binding commit")
	}}
	service := NewService(repository, nil, nil, log, nil, false)
	service.SetFormatterRuntime(runtime)
	_, err = service.ActivateFormatter(context.Background(), "alice", "mail", "", &models.FormatterBinding{PluginID: "formatter", InstallationID: "install", FormatterKey: "text", ConfigurationRevision: "snapshot", Digest: "digest"})
	require.NoError(t, err)
	require.False(t, runtime.held, "commit completion must release the installation")
}
