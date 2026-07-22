package github

import (
	"crypto/sha256"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	maxDeploymentAppManifestConversionCodeLength = 1024
	maxDeploymentAppProviderErrorLength          = 256
)

func (c *Controller) httpListAppRegistrations(ctx *gin.Context) {
	result, err := c.service.ListAppRegistrationCatalog(
		ctx.Request.Context(), currentGitHubUserID(ctx), ctx.Query("workspace_id"),
	)
	if err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) httpStartAppRegistrationManifest(ctx *gin.Context) {
	var request AppRegistrationManifestStartRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		writeAppRegistrationInvalidRequest(ctx)
		return
	}
	result, err := c.service.StartAppRegistrationManifest(
		ctx.Request.Context(), currentGitHubUserID(ctx), request,
	)
	if err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) httpCompleteAppRegistrationManifest(ctx *gin.Context) {
	state, code, providerError := ctx.Query("state"), ctx.Query("code"), ctx.Query("error")
	if !validGitHubCallbackState(state) ||
		len(code) > maxDeploymentAppManifestConversionCodeLength ||
		len(providerError) > maxDeploymentAppProviderErrorLength {
		redirectAppRegistrationCallback(ctx, "", "github_app_invalid_callback")
		return
	}
	result, err := c.service.CompleteAppRegistrationManifest(
		ctx.Request.Context(), ctx.Param("registrationId"), AppRegistrationManifestCallback{
			State: state, Code: code, Error: providerError,
		},
	)
	if err != nil {
		workspaceID := appRegistrationFlowWorkspace(ctx, c.service, state)
		redirectAppRegistrationCallback(ctx, workspaceID, githubAuthErrorCode(err))
		return
	}
	redirectAppRegistrationCallback(ctx, result.WorkspaceID, "app_registered")
}

func appRegistrationFlowWorkspace(ctx *gin.Context, service *Service, state string) string {
	if service == nil || service.store == nil || state == "" {
		return ""
	}
	digest := stateDigest(state)
	flow, err := service.store.GetDeploymentAppRegistrationFlow(ctx.Request.Context(), digest)
	if err != nil || flow == nil {
		return ""
	}
	return flow.WorkspaceID
}

func stateDigest(state string) string {
	digest := sha256Digest(strings.TrimSpace(state))
	return stateDigestString(digest)
}

func sha256Digest(value string) [32]byte {
	return sha256.Sum256([]byte(value))
}

func (c *Controller) httpImportAppRegistration(ctx *gin.Context) {
	var request AppRegistrationImportRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		writeAppRegistrationInvalidRequest(ctx)
		return
	}
	registration, err := c.service.ImportAppRegistration(
		ctx.Request.Context(), currentGitHubUserID(ctx), request,
	)
	if err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusCreated, registration)
}

func (c *Controller) httpPrepareAppRegistrationImport(ctx *gin.Context) {
	var request AppRegistrationImportPrepareRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		writeAppRegistrationInvalidRequest(ctx)
		return
	}
	preparation, err := c.service.PrepareAppRegistrationImport(
		ctx.Request.Context(), currentGitHubUserID(ctx), request,
	)
	if err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusCreated, preparation)
}

func (c *Controller) httpRenameAppRegistration(ctx *gin.Context) {
	var request struct {
		DisplayName string `json:"display_name"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil {
		writeAppRegistrationInvalidRequest(ctx)
		return
	}
	registration, err := c.service.RenameAppRegistration(
		ctx.Request.Context(), currentGitHubUserID(ctx), ctx.Param("registrationId"), request.DisplayName,
	)
	if err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, registration)
}

func (c *Controller) httpDeleteAppRegistration(ctx *gin.Context) {
	err := c.service.DeleteAppRegistration(
		ctx.Request.Context(), currentGitHubUserID(ctx), ctx.Param("registrationId"),
	)
	if err != nil {
		var inUse *AppRegistrationInUseError
		if errors.As(err, &inUse) {
			ctx.JSON(http.StatusConflict, gin.H{
				"code": "github_app_registration_in_use", "error": inUse.Error(),
				"binding_count": inUse.BindingCount,
			})
			return
		}
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func writeAppRegistrationInvalidRequest(ctx *gin.Context) {
	ctx.JSON(http.StatusBadRequest, gin.H{
		"code": "github_app_invalid_request", "error": "invalid GitHub App registration request",
	})
}

func redirectAppRegistrationCallback(ctx *gin.Context, workspaceID, result string) {
	query := url.Values{"github_result": []string{result}}
	if workspaceID != "" {
		query.Set("workspace_id", workspaceID)
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Referrer-Policy", "no-referrer")
	ctx.Redirect(http.StatusSeeOther, "/settings/integrations/github?"+query.Encode())
}

func deploymentAppHTTPError(err error) (int, string, bool) {
	switch {
	case errors.Is(err, ErrDeploymentAppOperatorRequired):
		return http.StatusForbidden, "github_app_operator_required", true
	case errors.Is(err, ErrDeploymentAppRegistrationCancelled):
		return http.StatusBadRequest, "github_app_registration_cancelled", true
	case errors.Is(err, ErrAppRegistrationNotFound):
		return http.StatusNotFound, "github_app_registration_not_found", true
	case errors.Is(err, ErrDeploymentAppInUse):
		return http.StatusConflict, "github_app_registration_in_use", true
	case errors.Is(err, ErrDeploymentAppManifestOwnerInvalid),
		errors.Is(err, ErrAppRegistrationIDInvalid),
		errors.Is(err, ErrAppRegistrationVisibilityInvalid):
		return http.StatusBadRequest, "github_app_invalid_request", true
	case errors.Is(err, ErrPublicGitHubBaseURLInvalid),
		errors.Is(err, ErrPublicGitHubBaseURLNotGlobal),
		errors.Is(err, ErrPublicGitHubBaseURLUnresolvable):
		return http.StatusBadRequest, "github_app_public_url_invalid", true
	case errors.Is(err, ErrDeploymentAppManifestStateUnavailable),
		errors.Is(err, ErrDeploymentAppIdentityMismatch),
		errors.Is(err, ErrDeploymentAppPolicyMismatch):
		return http.StatusBadRequest, "github_app_invalid_callback", true
	case errors.Is(err, ErrAppRegistrationImportPreparationUnavailable):
		return http.StatusBadRequest, "github_app_import_preparation_invalid", true
	}
	var importError *AppRegistrationImportError
	if errors.As(err, &importError) {
		code := "github_app_" + string(importError.Code)
		if importError.Code == AppRegistrationImportAlreadyRegistered {
			return http.StatusConflict, "github_app_already_registered", true
		}
		if importError.Code == AppRegistrationImportInvalidRequest {
			return http.StatusBadRequest, "github_app_invalid_request", true
		}
		if importError.Code == AppRegistrationImportPersistenceFailed {
			return http.StatusInternalServerError, code, true
		}
		return http.StatusUnprocessableEntity, code, true
	}
	var conversionError *ManifestConversionError
	if errors.As(err, &conversionError) {
		return http.StatusBadGateway, string(conversionError.Code), true
	}
	return 0, "", false
}
