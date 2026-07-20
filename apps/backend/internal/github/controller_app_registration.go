package github

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

const (
	maxDeploymentAppManifestConversionCodeLength = 1024
	maxDeploymentAppProviderErrorLength          = 256
)

func (c *Controller) httpGetDeploymentAppRegistration(ctx *gin.Context) {
	status, err := c.service.DeploymentAppRegistrationStatus(
		ctx.Request.Context(), currentGitHubUserID(ctx),
	)
	if err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, status)
}

func (c *Controller) httpStartDeploymentAppRegistration(ctx *gin.Context) {
	var request DeploymentAppRegistrationStartRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code": "github_app_invalid_request", "error": "invalid GitHub App registration request",
		})
		return
	}
	result, err := c.service.StartDeploymentAppRegistration(
		ctx.Request.Context(), currentGitHubUserID(ctx), request,
	)
	if err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) httpCompleteDeploymentAppRegistration(ctx *gin.Context) {
	state := ctx.Query("state")
	code := ctx.Query("code")
	providerError := ctx.Query("error")
	if !validGitHubCallbackState(state) ||
		len(code) > maxDeploymentAppManifestConversionCodeLength ||
		len(providerError) > maxDeploymentAppProviderErrorLength {
		redirectDeploymentAppCallback(ctx, "github_app_invalid_callback")
		return
	}
	_, err := c.service.CompleteDeploymentAppRegistration(
		ctx.Request.Context(),
		DeploymentAppRegistrationCallback{
			State: state, Code: code, Error: providerError,
		},
	)
	if err != nil {
		redirectDeploymentAppCallback(ctx, githubAuthErrorCode(err))
		return
	}
	redirectDeploymentAppCallback(ctx, "connected")
}

func (c *Controller) httpDeleteDeploymentAppRegistration(ctx *gin.Context) {
	if err := c.service.DeleteDeploymentAppRegistration(
		ctx.Request.Context(), currentGitHubUserID(ctx),
	); err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func redirectDeploymentAppCallback(ctx *gin.Context, result string) {
	query := url.Values{"github_app_result": []string{result}}
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Referrer-Policy", "no-referrer")
	ctx.Redirect(http.StatusSeeOther, "/settings/system/github-app?"+query.Encode())
}

func deploymentAppHTTPError(err error) (int, string, bool) {
	switch {
	case errors.Is(err, ErrDeploymentAppOperatorRequired):
		return http.StatusForbidden, "github_app_operator_required", true
	case errors.Is(err, ErrDeploymentAppEnvironmentReadOnly):
		return http.StatusConflict, "github_app_environment_read_only", true
	case errors.Is(err, ErrDeploymentAppRegistrationCancelled):
		return http.StatusBadRequest, "github_app_registration_cancelled", true
	case errors.Is(err, ErrDeploymentAppInUse):
		return http.StatusConflict, "github_app_in_use", true
	case errors.Is(err, ErrDeploymentAppManifestOwnerInvalid):
		return http.StatusBadRequest, "github_app_owner_invalid", true
	case errors.Is(err, ErrPublicGitHubBaseURLInvalid),
		errors.Is(err, ErrPublicGitHubBaseURLNotGlobal),
		errors.Is(err, ErrPublicGitHubBaseURLUnresolvable):
		return http.StatusBadRequest, "github_app_public_url_invalid", true
	case errors.Is(err, ErrDeploymentAppManifestStateUnavailable),
		errors.Is(err, ErrDeploymentAppIdentityMismatch),
		errors.Is(err, ErrDeploymentAppPolicyMismatch):
		return http.StatusBadRequest, "github_app_invalid_callback", true
	}
	var conversionError *ManifestConversionError
	if errors.As(err, &conversionError) {
		return http.StatusBadGateway, string(conversionError.Code), true
	}
	return 0, "", false
}
