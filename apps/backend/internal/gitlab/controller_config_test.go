package gitlab

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type configTestSecrets struct {
	values                  map[string]string
	setFailures             int
	deleteFailures          int
	mutateBeforeSetError    bool
	mutateBeforeDeleteError bool
}

func (s *configTestSecrets) Reveal(_ context.Context, id string) (string, error) {
	return s.values[id], nil
}
func (s *configTestSecrets) Set(_ context.Context, id, _ string, value string) error {
	if s.setFailures > 0 {
		s.setFailures--
		if s.mutateBeforeSetError {
			s.values[id] = value
		}
		return errors.New("injected secret set failure")
	}
	s.values[id] = value
	return nil
}
func (s *configTestSecrets) Delete(_ context.Context, id string) error {
	if s.deleteFailures > 0 {
		s.deleteFailures--
		if s.mutateBeforeDeleteError {
			delete(s.values, id)
		}
		return errors.New("injected secret delete failure")
	}
	delete(s.values, id)
	return nil
}
func (s *configTestSecrets) Exists(_ context.Context, id string) (bool, error) {
	_, ok := s.values[id]
	return ok, nil
}

func TestControllerConfigReturnsNoContentWhenWorkspaceIsUnconfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	svc := NewService(DefaultHost, NewNoopClient(DefaultHost), AuthMethodNone, nil, newTestLogger(t))
	svc.SetStore(store)
	router := gin.New()
	NewController(svc, newTestLogger(t)).RegisterHTTPRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/gitlab/config?workspace_id=workspace-a", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusNoContent, res.Body.String())
	}
}

func TestControllerStatusRejectsMissingWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := NewService(DefaultHost, NewNoopClient(DefaultHost), AuthMethodNone, nil, newTestLogger(t))
	router := gin.New()
	NewController(svc, newTestLogger(t)).RegisterHTTPRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/gitlab/status", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", res.Code, res.Body.String())
	}
}

func TestControllerConfigSavesOnlyRequestedWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gitlabServer := httptest.NewServer(userHandler(func(token string) bool { return token == "workspace-token" }))
	t.Cleanup(gitlabServer.Close)

	store := newTestStore(t)
	seedWorkspace(t, store, "workspace-a")
	seedWorkspace(t, store, "workspace-b")
	svc := NewService(DefaultHost, NewNoopClient(DefaultHost), AuthMethodNone, nil, newTestLogger(t))
	svc.SetStore(store)
	svc.SetWorkspaceSecretStore(&configTestSecrets{values: make(map[string]string)})
	router := gin.New()
	NewController(svc, newTestLogger(t)).RegisterHTTPRoutes(router)

	body := []byte(`{"host":"` + gitlabServer.URL + `/","auth_method":"pat","token":"workspace-token"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/gitlab/config?workspace_id=workspace-a", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", res.Code, res.Body.String())
	}
	cfgA, err := store.GetConfigForWorkspace(context.Background(), "workspace-a")
	if err != nil || cfgA == nil {
		t.Fatalf("workspace-a config = %#v, err=%v", cfgA, err)
	}
	if cfgA.Host != gitlabServer.URL {
		t.Fatalf("workspace-a host = %q, want %q", cfgA.Host, gitlabServer.URL)
	}
	cfgB, err := store.GetConfigForWorkspace(context.Background(), "workspace-b")
	if err != nil {
		t.Fatalf("workspace-b config: %v", err)
	}
	if cfgB != nil {
		t.Fatalf("workspace-b config leaked: %#v", cfgB)
	}
}
