package plugins

import (
	"errors"
	"net/http"
	"os"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins/store"
)

func registerPluginRoutesWithIdentity(t *testing.T, svc *Service, identity authn.Identity) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(ctx *gin.Context) {
		authn.SetOnGin(ctx, identity)
		ctx.Next()
	})
	RegisterRoutes(router, svc, nil, testLogger(t))
	return router
}

func registerAdminPluginRoutes(t *testing.T, svc *Service) *gin.Engine {
	t.Helper()
	return registerPluginRoutesWithIdentity(t, svc, authn.Identity{
		UserID: "admin-1",
		Role:   authn.RoleAdmin,
	})
}

func registerMemberPluginRoutes(t *testing.T, svc *Service) *gin.Engine {
	t.Helper()
	return registerPluginRoutesWithIdentity(t, svc, authn.Identity{
		UserID: "member-1",
		Role:   authn.RoleMember,
	})
}

func requireMemberForbidden(t *testing.T, recStatus int, responseBody string) {
	t.Helper()
	if recStatus != http.StatusForbidden {
		t.Errorf("status = %d, want 403, body=%s", recStatus, responseBody)
	}
}

func TestUpdateConfigHandlerRejectsMemberBeforePersistenceOrRestart(t *testing.T) {
	svc, _, runtime := newTestService(t)
	router := registerMemberPluginRoutes(t, svc)
	installTestPlugin(t, svc, "kandev-plugin-slack")
	startCallsBefore := runtime.startCallCount("kandev-plugin-slack")

	rec := doRequest(router, http.MethodPatch, "/api/plugins/kandev-plugin-slack", `{"config":{"default_channel":"#member"}}`, nil)
	requireMemberForbidden(t, rec.Code, rec.Body.String())
	config, err := svc.GetMaskedConfig("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("GetMaskedConfig: %v", err)
	}
	if len(config) != 0 {
		t.Errorf("member config persisted: %v", config)
	}
	if runtime.stopped("kandev-plugin-slack") {
		t.Error("member config request stopped the running plugin")
	}
	if got := runtime.startCallCount("kandev-plugin-slack"); got != startCallsBefore {
		t.Errorf("start calls = %d, want %d", got, startCallsBefore)
	}
}

func TestSyncHandlerRejectsMemberBeforeInstallingDroppedPackage(t *testing.T) {
	svc, pluginsDir, _, runtime := newTestServiceWithDir(t)
	router := registerMemberPluginRoutes(t, svc)
	tarPath := dropTarball(t, pluginsDir, "kandev-plugin-drop", "kandev-plugin-drop-1.0.0.tar.gz")

	rec := doRequest(router, http.MethodPost, "/api/plugins/sync", "", nil)
	requireMemberForbidden(t, rec.Code, rec.Body.String())
	if _, err := os.Stat(tarPath); err != nil {
		t.Errorf("member sync consumed dropped package: %v", err)
	}
	if _, err := svc.Get("kandev-plugin-drop"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("member sync registered dropped package, error = %v", err)
	}
	if runtime.Running("kandev-plugin-drop") {
		t.Error("member sync started dropped package")
	}
}

func TestUpdateSettingsHandlerRejectsMemberBeforePersistence(t *testing.T) {
	router, svc := newTestRouterWithSettingsIdentity(t, authn.Identity{
		UserID: "member-1",
		Role:   authn.RoleMember,
	})

	rec := doRequest(router, http.MethodPut, "/api/plugins/settings",
		`{"auto_update_default":true}`, map[string]string{"Content-Type": "application/json"})
	requireMemberForbidden(t, rec.Code, rec.Body.String())
	got, err := svc.AutoUpdateDefault()
	if err != nil {
		t.Fatalf("AutoUpdateDefault: %v", err)
	}
	if got {
		t.Error("member changed the global auto-update default")
	}
}

func TestSetAutoUpdateHandlerRejectsMemberBeforePersistence(t *testing.T) {
	router, svc := newTestRouterWithSettingsIdentity(t, authn.Identity{
		UserID: "member-1",
		Role:   authn.RoleMember,
	})
	installTestPlugin(t, svc, "kandev-plugin-slack")

	rec := doRequest(router, http.MethodPut, "/api/plugins/kandev-plugin-slack/auto-update",
		`{"auto_update":true}`, map[string]string{"Content-Type": "application/json"})
	requireMemberForbidden(t, rec.Code, rec.Body.String())
	got, err := svc.Get("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.AutoUpdate != nil {
		t.Errorf("member persisted auto-update override: %v", *got.AutoUpdate)
	}
}

func TestUninstallHandlerRejectsMemberBeforeRemovalOrStop(t *testing.T) {
	svc, _, runtime := newTestService(t)
	router := registerMemberPluginRoutes(t, svc)
	installTestPlugin(t, svc, "kandev-plugin-slack")

	rec := doRequest(router, http.MethodDelete, "/api/plugins/kandev-plugin-slack", "", nil)
	requireMemberForbidden(t, rec.Code, rec.Body.String())
	got, err := svc.Get("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("member uninstalled plugin: %v", err)
	}
	if got.Status != StatusActive || !runtime.Running(got.ID) {
		t.Errorf("plugin changed after member uninstall: status=%q running=%v", got.Status, runtime.Running(got.ID))
	}
	if runtime.stopped(got.ID) {
		t.Error("member uninstall stopped the plugin")
	}
}

func TestEnableHandlerRejectsMemberBeforeStart(t *testing.T) {
	svc, _, runtime := newTestService(t)
	router := registerMemberPluginRoutes(t, svc)
	installTestPlugin(t, svc, "kandev-plugin-slack")
	if err := svc.Disable("kandev-plugin-slack"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	startCallsBefore := runtime.startCallCount("kandev-plugin-slack")

	rec := doRequest(router, http.MethodPost, "/api/plugins/kandev-plugin-slack/enable", "", nil)
	requireMemberForbidden(t, rec.Code, rec.Body.String())
	got, err := svc.Get("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusDisabled || runtime.Running(got.ID) {
		t.Errorf("plugin changed after member enable: status=%q running=%v", got.Status, runtime.Running(got.ID))
	}
	if calls := runtime.startCallCount(got.ID); calls != startCallsBefore {
		t.Errorf("start calls = %d, want %d", calls, startCallsBefore)
	}
}

func TestDisableHandlerRejectsMemberBeforeStop(t *testing.T) {
	svc, _, runtime := newTestService(t)
	router := registerMemberPluginRoutes(t, svc)
	installTestPlugin(t, svc, "kandev-plugin-slack")

	rec := doRequest(router, http.MethodPost, "/api/plugins/kandev-plugin-slack/disable", "", nil)
	requireMemberForbidden(t, rec.Code, rec.Body.String())
	got, err := svc.Get("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusActive || !runtime.Running(got.ID) {
		t.Errorf("plugin changed after member disable: status=%q running=%v", got.Status, runtime.Running(got.ID))
	}
	if runtime.stopped(got.ID) {
		t.Error("member disable stopped the plugin")
	}
}
