package system

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
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
	max          int
	mergeEnabled bool
}

func (t *queueSettingsTarget) MaxPerSession() int     { return t.max }
func (t *queueSettingsTarget) SetMaxPerSession(n int) { t.max = n }
func (t *queueSettingsTarget) MergeEnabled() bool     { return t.mergeEnabled }
func (t *queueSettingsTarget) SetMergeEnabled(v bool) { t.mergeEnabled = v }

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
