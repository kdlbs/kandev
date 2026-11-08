package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func seedRemoteURLRoute(t *testing.T) *checkoutRouteFixture {
	t.Helper()
	fixtures := checkoutRoutePair(t)
	row, err := fixtures[0].repo.GetRepository(t.Context(), "route-repo")
	require.NoError(t, err)
	row.Name = "owner/repo"
	row.SourceType = "provider"
	row.Provider = "github"
	row.ProviderHost = "https://github.com"
	row.ProviderOwner = "owner"
	row.ProviderName = "repo"
	row.RemoteURL = "https://github.com/owner/old.git"
	require.NoError(t, fixtures[0].repo.UpdateRepository(t.Context(), row))
	return fixtures[0]
}

func postRemoteURLValidation(t *testing.T, router *gin.Engine, id, remoteURL string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/repositories/"+id+"/remote-url/validate",
		strings.NewReader(`{"remote_url":"`+remoteURL+`"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func dispatchRemoteURLValidation(t *testing.T, f *checkoutRouteFixture, id, remoteURL string) (*ws.Message, error) {
	t.Helper()
	request, err := ws.NewRequest("remote-url-validate", ws.ActionRepositoryValidateRemoteURL, map[string]any{
		"id": id, "remote_url": remoteURL,
	})
	require.NoError(t, err)
	return f.dispatch.Dispatch(context.Background(), request)
}

func TestRepositoryRemoteURLHTTP(t *testing.T) {
	f := seedRemoteURLRoute(t)
	router := f.router

	response := postRemoteURLValidation(t, router, "route-repo", "https://github.com/owner/repo.git")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var preview service.RepositoryRemoteURLPreview
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &preview))
	require.Equal(t, "https://github.com/owner/repo.git", preview.EffectiveRemoteURL)
	require.Equal(t, "locator_identity", preview.ValidationScope)
	require.True(t, preview.Changed)
	require.Equal(t, "https://github.com/owner/old.git", preview.CurrentRemoteURL)
	require.Empty(t, f.events.records())
	stored, err := f.repo.GetRepository(t.Context(), "route-repo")
	require.NoError(t, err)
	require.Equal(t, "https://github.com/owner/old.git", stored.RemoteURL)

	row, code := f.update(t, context.Background(), "HTTP", `{"remote_url":"git@github.com:owner/repo.git"}`)
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "git@github.com:owner/repo.git", row.RemoteURL)
	stored, err = f.repo.GetRepository(t.Context(), "route-repo")
	require.NoError(t, err)
	require.Equal(t, "git@github.com:owner/repo.git", stored.RemoteURL)
	require.Equal(t, "owner", stored.ProviderOwner)
	require.Equal(t, "repo", stored.ProviderName)
	events := f.events.records()
	require.Len(t, events, 1)
	require.Equal(t, "git@github.com:owner/repo.git", events[0].Data.(map[string]interface{})["remote_url"])

	read := httptest.NewRequest(http.MethodGet, "/api/v1/repositories/route-repo", nil)
	readResponse := httptest.NewRecorder()
	router.ServeHTTP(readResponse, read)
	require.Equal(t, http.StatusOK, readResponse.Code)
	var readback dto.RepositoryDTO
	require.NoError(t, json.Unmarshal(readResponse.Body.Bytes(), &readback))
	require.Equal(t, "git@github.com:owner/repo.git", readback.RemoteURL)

	for _, remoteURL := range []string{
		"https://token@github.com/owner/repo.git",
		"https://github.com/owner/other.git",
		"https://github.com/owner/repo/pull/1",
		"/srv/repositories/repo.git",
	} {
		response = postRemoteURLValidation(t, router, "route-repo", remoteURL)
		require.Equal(t, http.StatusBadRequest, response.Code, remoteURL)
		var body map[string]any
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
		require.NotContains(t, response.Body.String(), "token@")
	}
	require.Len(t, f.events.records(), 1)
}

func TestRepositoryRemoteURLHTTPPreservesExactIdentity(t *testing.T) {
	f := seedRemoteURLRoute(t)
	body := `{"remote_url":"https://github.com/owner/repo.git","source_type":" provider ","provider":" github ","provider_owner":" owner ","provider_name":" repo "}`
	row, code := f.update(t, context.Background(), "HTTP", body)
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "https://github.com/owner/repo.git", row.RemoteURL)
	stored, err := f.repo.GetRepository(t.Context(), "route-repo")
	require.NoError(t, err)
	require.Equal(t, "provider", stored.SourceType)
	require.Equal(t, "github", stored.Provider)
	require.Equal(t, "owner", stored.ProviderOwner)
	require.Equal(t, "repo", stored.ProviderName)
	require.Equal(t, "https://github.com", stored.ProviderHost)
}

func TestRepositoryRemoteURLWS(t *testing.T) {
	f := seedRemoteURLRoute(t)

	response, err := dispatchRemoteURLValidation(t, f, "route-repo", "https://github.com/owner/repo.git")
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	var preview service.RepositoryRemoteURLPreview
	require.NoError(t, json.Unmarshal(response.Payload, &preview))
	require.Equal(t, "https://github.com/owner/repo.git", preview.EffectiveRemoteURL)
	require.Empty(t, f.events.records())

	row, code := f.update(t, context.Background(), "WS", `{"remote_url":"https://github.com/owner/repo.git"}`)
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "https://github.com/owner/repo.git", row.RemoteURL)
	stored, err := f.repo.GetRepository(t.Context(), "route-repo")
	require.NoError(t, err)
	require.Equal(t, "https://github.com/owner/repo.git", stored.RemoteURL)

	request, err := ws.NewRequest("get", ws.ActionRepositoryGet, map[string]any{"id": "route-repo"})
	require.NoError(t, err)
	getResponse, err := f.dispatch.Dispatch(t.Context(), request)
	require.NoError(t, err)
	var readback dto.RepositoryDTO
	require.NoError(t, json.Unmarshal(getResponse.Payload, &readback))
	require.Equal(t, "https://github.com/owner/repo.git", readback.RemoteURL)

	failure, err := dispatchRemoteURLValidation(t, f, "route-repo", "https://github.com/owner/other.git")
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, failure.Type)
	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(failure.Payload, &payload))
	require.Equal(t, ws.ErrorCodeValidation, payload.Code)
}

func TestRepositoryRemoteURLReadOnlyWorkspace(t *testing.T) {
	f := seedRemoteURLRoute(t)
	workspace, err := f.repo.GetWorkspace(t.Context(), "route-ws")
	require.NoError(t, err)
	workspace.Name = "Improve Kandev"
	require.NoError(t, f.repo.UpdateWorkspace(t.Context(), workspace))

	response := postRemoteURLValidation(t, f.router, "route-repo", "https://github.com/owner/repo.git")
	require.Equal(t, http.StatusConflict, response.Code)

	_, code := f.update(t, context.Background(), "HTTP", `{"remote_url":"https://github.com/owner/repo.git"}`)
	require.Equal(t, http.StatusConflict, code)

	failure, err := dispatchRemoteURLValidation(t, f, "route-repo", "https://github.com/owner/repo.git")
	require.NoError(t, err)
	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(failure.Payload, &payload))
	require.Equal(t, ws.ErrorCodeConflict, payload.Code)
}

func TestRepositoryRemoteURLUnsafeLegacyLocatorIsNotEchoed(t *testing.T) {
	f := seedRemoteURLRoute(t)
	stored, err := f.repo.GetRepository(t.Context(), "route-repo")
	require.NoError(t, err)
	stored.RemoteURL = "https://secret-token@github.com/owner/old.git"
	require.NoError(t, f.repo.UpdateRepository(t.Context(), stored))

	response := postRemoteURLValidation(t, f.router, "route-repo", "https://github.com/owner/repo.git")
	require.Equal(t, http.StatusOK, response.Code)
	require.NotContains(t, response.Body.String(), "secret-token")
	var preview service.RepositoryRemoteURLPreview
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &preview))
	require.Empty(t, preview.CurrentRemoteURL)
	require.True(t, preview.CurrentRemoteURLRedacted)
}

func TestRepositoryRemoteURLIdentityConflictHTTP(t *testing.T) {
	f := seedRemoteURLRoute(t)
	f.gate.remaining.Store(2)
	ctx := t.Context()
	done := make(chan int, 1)
	go func() {
		request := httptest.NewRequest(http.MethodPatch, "/api/v1/repositories/route-repo",
			strings.NewReader(`{"remote_url":"https://github.com/owner/repo.git"}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		f.router.ServeHTTP(response, request)
		done <- response.Code
	}()
	select {
	case <-f.gate.read:
	case <-ctx.Done():
		t.Fatal("validation snapshot read not observed")
	}
	stored, err := f.repo.GetRepository(ctx, "route-repo")
	require.NoError(t, err)
	stored.ProviderOwner = "other"
	require.NoError(t, f.repo.UpdateRepository(ctx, stored))
	close(f.gate.release)
	select {
	case code := <-done:
		require.Equal(t, http.StatusConflict, code)
	case <-ctx.Done():
		t.Fatal("conflicting update did not finish")
	}
	stored, err = f.repo.GetRepository(ctx, "route-repo")
	require.NoError(t, err)
	require.Equal(t, "other", stored.ProviderOwner)
	require.Equal(t, "https://github.com/owner/old.git", stored.RemoteURL)
}

func TestRepositoryRemoteURLMissingRepository(t *testing.T) {
	f := seedRemoteURLRoute(t)
	response := postRemoteURLValidation(t, f.router, "missing", "https://github.com/owner/repo.git")
	require.Equal(t, http.StatusNotFound, response.Code)
}
