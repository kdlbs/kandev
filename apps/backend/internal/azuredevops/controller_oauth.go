package azuredevops

import (
	"errors"
	"html/template"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/i18n"
)

const (
	oauthUnavailableCode     = "azure_devops_oauth_unavailable"
	oauthInvalidStateCode    = "azure_devops_oauth_invalid_state"
	oauthFailedCode          = "azure_devops_oauth_failed"
	oauthProviderCode        = "azure_devops_oauth_provider_unavailable"
	oauthReauthRequiredCode  = "azure_devops_reauth_required"
	oauthCallbackMessageType = "azure-devops-oauth-callback"
)

func registerOAuthRoutes(api *gin.RouterGroup, controller *Controller) {
	api.GET("/oauth/availability", controller.getOAuthAvailability)
	api.POST("/oauth/start", controller.startOAuth)
	api.GET("/oauth/callback", controller.oauthCallback)
	api.POST("/oauth/complete", controller.completeOAuth)
	api.POST("/oauth/sign-out", controller.signOutOAuth)
}

func (c *Controller) signOutOAuth(ctx *gin.Context) {
	cfg, err := c.service.SignOutOAuth(ctx.Request.Context(), workspaceID(ctx))
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, cfg)
}

func (c *Controller) getOAuthAvailability(ctx *gin.Context) {
	defaults := c.service.OAuthDefaults()
	ctx.JSON(http.StatusOK, gin.H{
		"available":          c.service.OAuthAvailable(),
		"defaultClientId":    defaults.ClientID,
		"defaultRedirectUrl": defaults.RedirectURL,
	})
}

func (c *Controller) startOAuth(ctx *gin.Context) {
	var request StartOAuthRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	result, err := c.service.StartOAuth(ctx.Request.Context(), workspaceID(ctx), &request)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) completeOAuth(ctx *gin.Context) {
	var request CompleteOAuthRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	cfg, err := c.service.CompleteOAuth(ctx.Request.Context(), workspaceID(ctx), &request)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, cfg)
}

// oauthCallbackTemplate forwards Microsoft's redirect parameters to the
// Kandev window that opened the popup. It reads them from its own URL, posts
// only to its own origin, and never echoes them into the markup.
var oauthCallbackTemplate = template.Must(template.New("azure-devops-oauth-callback").Parse(`<!doctype html>
<html lang="{{.Locale}}"><head><meta charset="utf-8"><title>{{.Title}}</title></head>
<body><p>{{.Instructions}}</p>
<script>
(function () {
  var params = new URLSearchParams(window.location.search);
  if (!window.opener) { return; }
  window.opener.postMessage({
    type: {{.MessageType}},
    code: params.get("code") || "",
    state: params.get("state") || "",
    error: params.get("error") || ""
  }, window.location.origin);
  window.setTimeout(function () { window.close(); }, 100);
})();
</script></body></html>`))

func (c *Controller) oauthCallback(ctx *gin.Context) {
	locale := i18n.FromRequest(ctx.Request)
	ctx.Header("Content-Type", "text/html; charset=utf-8")
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Referrer-Policy", "no-referrer")
	ctx.Status(http.StatusOK)
	_ = oauthCallbackTemplate.Execute(ctx.Writer, map[string]string{
		"Locale":       locale,
		"Title":        i18n.T(locale, "azureDevOps.oauthCallback.title"),
		"Instructions": i18n.T(locale, "azureDevOps.oauthCallback.instructions"),
		"MessageType":  oauthCallbackMessageType,
	})
}

// oauthErrorStatus maps Microsoft sign-in failures to HTTP responses.
func oauthErrorStatus(err error) (status int, code string, ok bool) {
	var entraErr *EntraError
	switch {
	case errors.Is(err, ErrOAuthUnavailable):
		return http.StatusConflict, oauthUnavailableCode, true
	case errors.Is(err, ErrOAuthFlowInvalid):
		return http.StatusBadRequest, oauthInvalidStateCode, true
	case errors.Is(err, ErrReauthRequired):
		return http.StatusUnauthorized, oauthReauthRequiredCode, true
	case errors.Is(err, ErrOAuthProviderUnavailable):
		return http.StatusBadGateway, oauthProviderCode, true
	case errors.As(err, &entraErr):
		return http.StatusBadRequest, oauthFailedCode, true
	}
	return 0, "", false
}
