package system

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/system/database"
	"github.com/kandev/kandev/internal/system/frontenderrors"
	"github.com/kandev/kandev/internal/system/queuesettings"
	"github.com/kandev/kandev/internal/system/sleepinhibition"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

func TestRegisterRoutesAllowsMemberFrontendErrorReports(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: "member-1", Role: authn.RoleMember})
		c.Next()
	})
	service := &Service{FrontendErrors: frontenderrors.New(log, nil)}
	service.RegisterRoutes(router, log)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/system/logs/frontend-errors",
		bytes.NewBufferString(`{"source":"sonner","title":"visible error"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("member report status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestRegisterRoutesAllowsMemberToRetryDatabaseStats(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	databaseService := database.NewService(nil, filepath.Join(t.TempDir(), "kandev.db"), database.ResetDirs{}, nil, nil)
	t.Cleanup(databaseService.StopBackground)
	router := systemRouterForRole(authn.RoleMember)
	(&Service{Database: databaseService}).RegisterRoutes(router, log)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/system/database/refresh", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("member retry status = %d, want 204; body=%s", response.Code, response.Body.String())
	}
}

func TestRegisterRoutesMessageQueueSettingsPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	target := &queueSettingsTarget{max: 10}
	queueService := queuesettings.NewService(
		queuesettings.NewStore(&queueSettingsRawStore{}), target,
		func() queuesettings.Environment { return queuesettings.Environment{} }, log,
	)

	memberRouter := systemRouterForRole(authn.RoleMember)
	(&Service{MessageQueue: queueService}).RegisterRoutes(memberRouter, log)
	getResponse := httptest.NewRecorder()
	memberRouter.ServeHTTP(getResponse, httptest.NewRequest(
		http.MethodGet, "/api/v1/system/message-queue/settings", nil,
	))
	if getResponse.Code != http.StatusOK {
		t.Fatalf("member GET status = %d, want 200; body=%s", getResponse.Code, getResponse.Body.String())
	}
	patchResponse := httptest.NewRecorder()
	memberRouter.ServeHTTP(patchResponse, queueSettingsPatchRequest(4))
	if patchResponse.Code != http.StatusForbidden {
		t.Fatalf("member PATCH status = %d, want 403; body=%s", patchResponse.Code, patchResponse.Body.String())
	}

	adminRouter := systemRouterForRole(authn.RoleAdmin)
	(&Service{MessageQueue: queueService}).RegisterRoutes(adminRouter, log)
	adminResponse := httptest.NewRecorder()
	adminRouter.ServeHTTP(adminResponse, queueSettingsPatchRequest(4))
	if adminResponse.Code != http.StatusOK || target.max != 4 {
		t.Fatalf("admin PATCH status=%d target=%d body=%s", adminResponse.Code, target.max, adminResponse.Body.String())
	}
}

func TestRegisterRoutesSleepInhibitionPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	service := sleepinhibition.NewService(
		sleepinhibition.NewStore(&systemSleepRawStore{}),
		systemSleepReader{},
		systemSleepInhibitor{},
		bus.NewMemoryEventBus(nil),
		nil,
	)

	memberRouter := systemRouterForRole(authn.RoleMember)
	(&Service{SleepInhibition: service}).RegisterRoutes(memberRouter, log)
	getResponse := httptest.NewRecorder()
	memberRouter.ServeHTTP(getResponse, httptest.NewRequest(
		http.MethodGet, "/api/v1/system/sleep-inhibition", nil,
	))
	if getResponse.Code != http.StatusOK {
		t.Fatalf("member GET status = %d, want 200; body=%s", getResponse.Code, getResponse.Body.String())
	}
	patchResponse := httptest.NewRecorder()
	memberRouter.ServeHTTP(patchResponse, sleepInhibitionPatchRequest(true))
	if patchResponse.Code != http.StatusForbidden {
		t.Fatalf("member PATCH status = %d, want 403; body=%s", patchResponse.Code, patchResponse.Body.String())
	}

	adminRouter := systemRouterForRole(authn.RoleAdmin)
	(&Service{SleepInhibition: service}).RegisterRoutes(adminRouter, log)
	adminResponse := httptest.NewRecorder()
	adminRouter.ServeHTTP(adminResponse, sleepInhibitionPatchRequest(true))
	if adminResponse.Code != http.StatusOK || !bytes.Contains(adminResponse.Body.Bytes(), []byte(`"enabled":true`)) {
		t.Fatalf("admin PATCH status=%d body=%s", adminResponse.Code, adminResponse.Body.String())
	}
}

func TestRuntimeRecoverAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	target := &agentRuntimeRecoveryTargetFake{snapshot: agentctlclient.AvailabilitySnapshot{
		Status: "recovering", BootID: "boot-1", RuntimeEpoch: 7, Revision: 12, RecoveryID: "recovery-1",
	}}
	memberRouter := systemRouterForRole(authn.RoleMember)
	(&Service{AgentRuntimeRecovery: target}).RegisterRoutes(memberRouter, log)
	memberResponse := httptest.NewRecorder()
	memberRouter.ServeHTTP(memberResponse, validAgentRuntimeRetryRequest())
	if memberResponse.Code != http.StatusForbidden || target.calls != 0 {
		t.Fatalf("member retry status=%d calls=%d body=%s", memberResponse.Code, target.calls, memberResponse.Body.String())
	}

	adminRouter := systemRouterForRole(authn.RoleAdmin)
	(&Service{AgentRuntimeRecovery: target}).RegisterRoutes(adminRouter, log)
	adminResponse := httptest.NewRecorder()
	adminRouter.ServeHTTP(adminResponse, validAgentRuntimeRetryRequest())
	if adminResponse.Code != http.StatusAccepted || target.calls != 1 {
		t.Fatalf("admin retry status=%d calls=%d body=%s", adminResponse.Code, target.calls, adminResponse.Body.String())
	}
	if target.bootID != "boot-1" || target.epoch != 7 || target.revision != 12 || target.requestID != "request-1" {
		t.Fatalf("retry fences = (%q, %d, %d, %q)", target.bootID, target.epoch, target.revision, target.requestID)
	}
	if !bytes.Contains(adminResponse.Body.Bytes(), []byte(`"recovery_id":"recovery-1"`)) {
		t.Fatalf("accepted snapshot missing recovery id: %s", adminResponse.Body.String())
	}

	target.err = agentctlclient.ErrRecoveryConflict
	staleResponse := httptest.NewRecorder()
	adminRouter.ServeHTTP(staleResponse, validAgentRuntimeRetryRequest())
	if staleResponse.Code != http.StatusConflict || !bytes.Contains(staleResponse.Body.Bytes(), []byte(`"error_code":"stale_runtime_snapshot"`)) {
		t.Fatalf("stale retry status=%d body=%s", staleResponse.Code, staleResponse.Body.String())
	}
}

func TestAgentRuntimeRecoveryRetryRejectsInvalidSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	target := &agentRuntimeRecoveryTargetFake{}
	router := systemRouterForRole(authn.RoleAdmin)
	(&Service{AgentRuntimeRecovery: target}).RegisterRoutes(router, log)
	for _, body := range []string{
		`{"boot_id":"boot-1","runtime_epoch":7,"request_id":"request-1"}`,
		`{"boot_id":"boot-1","runtime_epoch":7,"revision":12,"request_id":"request-1","extra":true}`,
		`{"boot_id":"boot-1","runtime_epoch":7,"revision":12,"request_id":"request-1"} {}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/system/agent-runtime/retry", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("body %q status=%d body=%s, want 400", body, response.Code, response.Body.String())
		}
	}
	if target.calls != 0 {
		t.Fatalf("invalid requests reached recovery target %d times", target.calls)
	}
}

func validAgentRuntimeRetryRequest() *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/system/agent-runtime/retry", bytes.NewBufferString(
		`{"boot_id":"boot-1","runtime_epoch":7,"revision":12,"request_id":"request-1"}`,
	))
	request.Header.Set("Content-Type", "application/json")
	return request
}

type agentRuntimeRecoveryTargetFake struct {
	snapshot  agentctlclient.AvailabilitySnapshot
	err       error
	calls     int
	bootID    string
	epoch     uint64
	revision  uint64
	requestID string
}

func (target *agentRuntimeRecoveryTargetFake) RetryAtRevision(
	_ context.Context,
	bootID string,
	epoch uint64,
	revision uint64,
	requestID string,
) (agentctlclient.AvailabilitySnapshot, error) {
	target.calls++
	target.bootID, target.epoch, target.revision, target.requestID = bootID, epoch, revision, requestID
	return target.snapshot, target.err
}

func systemRouterForRole(role authn.Role) *gin.Engine {
	router := gin.New()
	router.Use(func(ctx *gin.Context) {
		authn.SetOnGin(ctx, authn.Identity{UserID: "user-1", Role: role})
		ctx.Next()
	})
	return router
}

func queueSettingsPatchRequest(max int) *http.Request {
	body := bytes.NewBufferString(`{"max_per_session":` + fmt.Sprint(max) + `}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/system/message-queue/settings", body)
	request.Header.Set("Content-Type", "application/json")
	return request
}

func sleepInhibitionPatchRequest(enabled bool) *http.Request {
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/system/sleep-inhibition", bytes.NewBufferString(fmt.Sprintf(`{"enabled":%t}`, enabled)))
	request.Header.Set("Content-Type", "application/json")
	return request
}

type queueSettingsRawStore struct {
	raw   []byte
	found bool
}

func (s *queueSettingsRawStore) Get(context.Context, string) ([]byte, bool, error) {
	return s.raw, s.found, nil
}

func (s *queueSettingsRawStore) Save(_ context.Context, _ string, value []byte) error {
	s.raw = append([]byte(nil), value...)
	s.found = true
	return nil
}

type queueSettingsTarget struct {
	max              int
	mergeEnabled     bool
	autoMergeEnabled bool
}

func (t *queueSettingsTarget) MaxPerSession() int         { return t.max }
func (t *queueSettingsTarget) SetMaxPerSession(n int)     { t.max = n }
func (t *queueSettingsTarget) MergeEnabled() bool         { return t.mergeEnabled }
func (t *queueSettingsTarget) SetMergeEnabled(v bool)     { t.mergeEnabled = v }
func (t *queueSettingsTarget) AutoMergeEnabled() bool     { return t.autoMergeEnabled }
func (t *queueSettingsTarget) SetAutoMergeEnabled(v bool) { t.autoMergeEnabled = v }

type systemSleepRawStore struct {
	raw   []byte
	found bool
}

func (s *systemSleepRawStore) Get(context.Context, string) ([]byte, bool, error) {
	return s.raw, s.found, nil
}

func (s *systemSleepRawStore) Save(_ context.Context, _ string, value []byte) error {
	s.raw = append([]byte(nil), value...)
	s.found = true
	return nil
}

type systemSleepReader struct{}

func (systemSleepReader) ListActiveTaskSessions(context.Context) ([]*models.TaskSession, error) {
	return nil, nil
}

type systemSleepInhibitor struct{}

func (systemSleepInhibitor) Platform() sleepinhibition.Platform { return sleepinhibition.PlatformOther }
func (systemSleepInhibitor) Supported() bool                    { return false }
func (systemSleepInhibitor) Acquire(context.Context) (sleepinhibition.Lease, error) {
	return nil, sleepinhibition.ErrUnsupported
}
