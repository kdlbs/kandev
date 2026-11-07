package plugins

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kdlbs/kandev/pluginsdk"
)

func boundFormatterManagement(ctx *gin.Context, scope string) context.CancelFunc {
	if scope != manifest.ActionScopeNotificationProvider {
		return func() {}
	}
	bounded, cancel := context.WithTimeout(ctx.Request.Context(), 5*time.Second)
	ctx.Request = ctx.Request.WithContext(bounded)
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 128*1024)
	return cancel
}

func actionResponseLimit(scope string) int {
	if scope == manifest.ActionScopeNotificationProvider {
		return 512 * 1024
	}
	return maxPluginActionResponseBytes
}

type formatterActionContext struct {
	InstallationID          string  `json:"installationId"`
	FormatterKey            string  `json:"formatterKey"`
	ExpectedBindingRevision *string `json:"expectedBindingRevision"`
	Mode                    string  `json:"mode"`
}

func (c *Controller) beginFormatterAction(ctx *gin.Context, record *store.Record, verified pluginsdk.VerifiedActionContext, observed *formatterActionContext) (func(), func() bool, bool) {
	if observed == nil || observed.ExpectedBindingRevision == nil || (observed.Mode != "active" && observed.Mode != "candidate") {
		ctx.JSON(http.StatusBadRequest, gin.H{actionErrorField: "formatter management context required"})
		return nil, nil, false
	}
	kind, err := c.svc.notificationProviders.NotificationProviderType(ctx.Request.Context(), verified.ActorID, verified.NotificationProviderID)
	if _, ok := declaredNotificationFormatter(record, observed.FormatterKey, kind, ctx.Param("key")); err != nil || !ok || record.InstallationID != observed.InstallationID {
		ctx.JSON(http.StatusConflict, gin.H{actionErrorField: "notification formatter changed"})
		return nil, nil, false
	}
	finish, err := c.svc.notificationProviders.BeginFormatterManagement(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{actionErrorField: "notification configuration capture in progress"})
		return nil, nil, false
	}
	release := finish
	check := func() bool {
		current, err := c.svc.Get(record.ID)
		if err != nil || current.Status != StatusActive || current.InstallationID != observed.InstallationID || !dispatchGeneration(record).matches(current) {
			return false
		}
		binding, err := c.svc.notificationProviders.NotificationFormatterBinding(ctx.Request.Context(), verified.ActorID, verified.NotificationProviderID)
		return err == nil && matchesFormatterAction(binding, record.ID, observed)
	}
	if !check() {
		release()
		ctx.JSON(http.StatusConflict, gin.H{actionErrorField: "notification formatter binding changed"})
		return nil, nil, false
	}
	return release, check, true
}

func matchesFormatterAction(binding *models.FormatterBinding, pluginID string, observed *formatterActionContext) bool {
	revision := ""
	if binding != nil {
		revision = binding.Revision
	}
	if revision != *observed.ExpectedBindingRevision {
		return false
	}
	return observed.Mode == "candidate" || binding != nil && binding.PluginID == pluginID && binding.InstallationID == observed.InstallationID && binding.FormatterKey == observed.FormatterKey
}
