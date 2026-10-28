package backendapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
)

func crashBarrierRequest(router *gin.Engine, path, body string) int {
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	return response.Code
}

func TestCrashBarrierRoutesAreRefusedOutsideTheE2EMock(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "")
	router := gin.New()
	registerE2ECrashBarrierRoutes(router)
	require.Equal(t, http.StatusNotFound, crashBarrierRequest(router, "/api/v1/e2e/crash-barriers/after_drain/arm", `{"mode":"block"}`))
	require.Equal(t, http.StatusNotFound, crashBarrierRequest(router, "/api/v1/e2e/crash-barriers/after_drain/release", ""))
}

func TestCrashBarrierRoutesArmConflictAndRelease(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "true")
	router := gin.New()
	registerE2ECrashBarrierRoutes(router)
	const arm = "/api/v1/e2e/crash-barriers/after_drain/arm"
	t.Cleanup(func() { agentruntime.ReleaseCrashBarrier(agentruntime.BarrierAfterDrain) })

	require.Equal(t, http.StatusNoContent, crashBarrierRequest(router, arm, `{"mode":"block"}`))
	require.Equal(t, http.StatusConflict, crashBarrierRequest(router, arm, `{"mode":"block"}`), "arming an armed barrier")
	require.Equal(t, http.StatusNoContent, crashBarrierRequest(router, "/api/v1/e2e/crash-barriers/after_drain/release", ""))
	require.Equal(t, http.StatusNoContent, crashBarrierRequest(router, "/api/v1/e2e/crash-barriers/after_drain/release", ""), "releasing an unarmed barrier")
	require.Equal(t, http.StatusNoContent, crashBarrierRequest(router, arm, `{"mode":"block"}`), "re-arming after release")
}

func TestCrashBarrierRoutesRejectBadNameAndMode(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "true")
	router := gin.New()
	registerE2ECrashBarrierRoutes(router)
	require.Equal(t, http.StatusBadRequest, crashBarrierRequest(router, "/api/v1/e2e/crash-barriers/no_such/arm", `{"mode":"block"}`))
	require.Equal(t, http.StatusBadRequest, crashBarrierRequest(router, "/api/v1/e2e/crash-barriers/after_drain/arm", `{"mode":"explode"}`))
	require.Equal(t, http.StatusBadRequest, crashBarrierRequest(router, "/api/v1/e2e/crash-barriers/after_drain/arm", `not json`))
}
