package plugins

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/notifications/models"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

type formatterProviderAccessFixture struct{ binding *models.FormatterBinding }

func (f *formatterProviderAccessFixture) NotificationFormatterBinding(context.Context, string, string) (*models.FormatterBinding, error) {
	return f.binding, nil
}

func (formatterProviderAccessFixture) NotificationProviderType(_ context.Context, owner, id string) (models.ProviderType, error) {
	if owner == "user-1" && id == "mail" {
		return models.ProviderTypeApprise, nil
	}
	return "", notificationstore.ErrProviderNotFound
}

func (formatterProviderAccessFixture) BeginFormatterManagement(context.Context) (func(), error) {
	return func() {}, nil
}

func TestFormatterManagementRequiresOwnedProviderBeforeDispatch(t *testing.T) {
	invoker := &recordingActionInvoker{response: &pluginsdk.PluginActionResponse{Body: []byte(`{}`)}}
	router, svc := newActionTestRouter(t, invoker)
	access := &formatterProviderAccessFixture{}
	svc.SetNotificationProviderAccess(access)
	yaml := fmt.Sprintf(`
id: formatter-fixture
api_version: 2
version: "1.0.0"
display_name: Formatter
notification_formatters:
  - key: text
    contract_version: 1
    provider_types: [apprise]
    management_actions: [prepare]
  - key: other-text
    contract_version: 1
    provider_types: [apprise]
    management_actions: [prepare]
actions:
  - key: prepare
    scope: notification_provider
    max_body_bytes: 131072
runtime:
  type: binary
  executables:
    %s-%s: server/plugin
`, runtime.GOOS, runtime.GOARCH)
	var archive bytes.Buffer
	require.NoError(t, pkgtartest.WritePackage(&archive, map[string][]byte{
		"manifest.yaml": []byte(yaml), "server/plugin": []byte("#!/bin/sh\necho fixture\n"),
	}))
	installed, err := svc.Install(t.Context(), &archive)
	require.NoError(t, err)
	for _, id := range []string{"foreign", "missing"} {
		response := doAuthenticatedActionRequest(router, "/api/plugins/formatter-fixture/actions/prepare", fmt.Sprintf(`{"notificationProviderId":%q,"body":{}}`, id))
		require.Equal(t, http.StatusNotFound, response.Code)
	}
	require.Zero(t, invoker.calls)
	missingContext := doAuthenticatedActionRequest(router, "/api/plugins/formatter-fixture/actions/prepare", `{"notificationProviderId":"mail","body":{}}`)
	require.Equal(t, http.StatusBadRequest, missingContext.Code)
	input := fmt.Sprintf(`{"notificationProviderId":"mail","notificationFormatter":{"installationId":%q,"formatterKey":"text","expectedBindingRevision":"","mode":"candidate"},"body":{"owner_id":"forged"}}`, installed.InstallationID)
	invoker.invoke = func(ctx context.Context, _ string, _ *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(deadline), 5*time.Second)
		return &pluginsdk.PluginActionResponse{Body: []byte(`{}`)}, nil
	}
	response := doAuthenticatedActionRequest(router, "/api/plugins/formatter-fixture/actions/prepare", input)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, "user-1", invoker.request.Context.ActorID)
	require.Equal(t, "mail", invoker.request.Context.NotificationProviderID)
	require.Equal(t, "text", invoker.request.Context.NotificationFormatterKey)
	emptyKey := strings.Replace(input, `"formatterKey":"text"`, `"formatterKey":""`, 1)
	response = doAuthenticatedActionRequest(router, "/api/plugins/formatter-fixture/actions/prepare", emptyKey)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Equal(t, 1, invoker.calls, "empty formatter selection must not dispatch")
	otherKey := strings.Replace(input, `"formatterKey":"text"`, `"formatterKey":"other-text"`, 1)
	response = doAuthenticatedActionRequest(router, "/api/plugins/formatter-fixture/actions/prepare", otherKey)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "other-text", invoker.request.Context.NotificationFormatterKey)
	access.binding = &models.FormatterBinding{Revision: "new-binding"}
	response = doAuthenticatedActionRequest(router, "/api/plugins/formatter-fixture/actions/prepare", input)
	require.Equal(t, http.StatusConflict, response.Code)
	require.Equal(t, 2, invoker.calls, "stale context must not dispatch")
	access.binding = nil
	invoker.invoke = func(context.Context, string, *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
		access.binding = &models.FormatterBinding{Revision: "changed-during-action"}
		return &pluginsdk.PluginActionResponse{Body: []byte(`{"configuration":"stale"}`)}, nil
	}
	response = doAuthenticatedActionRequest(router, "/api/plugins/formatter-fixture/actions/prepare", input)
	require.Equal(t, http.StatusConflict, response.Code)
	require.NotContains(t, response.Body.String(), "stale")
	access.binding = nil
	invoker.invoke = nil
	invoker.response.Body = []byte(strings.Repeat("x", 512*1024+1))
	response = doAuthenticatedActionRequest(router, "/api/plugins/formatter-fixture/actions/prepare", input)
	require.Equal(t, http.StatusBadGateway, response.Code)
}
