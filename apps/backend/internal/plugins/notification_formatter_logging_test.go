package plugins

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestFormatterActionFailureDoesNotLogPrivateContent(t *testing.T) {
	for _, scope := range []string{manifest.ActionScopeNotificationProvider, manifest.ActionScopeWorkspace} {
		t.Run(scope, func(t *testing.T) {
			const privateContent = "private template and receiver details"
			invoker := &recordingActionInvoker{err: errors.New(privateContent)}
			_, svc := newActionTestRouter(t, invoker)
			record := installTestPlugin(t, svc, "formatter-logging")
			record.Actions = []manifest.Action{{Key: "prepare", ResourceScope: scope, MaxBodyBytes: 1024}}
			record.NotificationFormatters = []manifest.NotificationFormatter{{
				Key: "text", ContractVersion: 1, ProviderTypes: []string{"apprise"}, ManagementActions: []string{"prepare"},
			}}
			svc.registry.Add(record)
			svc.SetNotificationProviderAccess(&formatterProviderAccessFixture{})
			core, entries := observer.New(zap.WarnLevel)
			log, err := logger.NewFromZap(zap.New(core))
			require.NoError(t, err)
			ctrl := &Controller{svc: svc, log: log, actionInvoker: invoker}
			router := gin.New()
			router.POST("/api/plugins/:id/actions/:key", ctrl.action)
			body := `{"workspaceId":"workspace-1","body":{}}`
			if scope == manifest.ActionScopeNotificationProvider {
				body = fmt.Sprintf(`{"notificationProviderId":"mail","notificationFormatter":{"installationId":%q,"formatterKey":"text","expectedBindingRevision":"","mode":"candidate"},"body":{}}`, record.InstallationID)
			}
			response := doAuthenticatedActionRequest(router, "/api/plugins/formatter-logging/actions/prepare", body)
			require.Equal(t, http.StatusServiceUnavailable, response.Code)
			require.Equal(t, 1, invoker.calls)
			require.NotContains(t, response.Body.String(), privateContent)
			failures := entries.FilterMessage("plugin action invocation failed").All()
			require.Len(t, failures, 1)
			fields := failures[0].ContextMap()
			require.Equal(t, record.ID, fields["plugin"])
			require.Equal(t, "prepare", fields["action"])
			if scope == manifest.ActionScopeNotificationProvider {
				require.Equal(t, "formatter_action_failed", fields["reason"])
				require.NotContains(t, fields, "error")
				require.NotContains(t, fmt.Sprint(failures), privateContent)
			} else {
				require.Equal(t, privateContent, fields["error"])
			}
		})
	}
}
