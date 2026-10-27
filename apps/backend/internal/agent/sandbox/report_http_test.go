package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/stretchr/testify/require"
)

// AC-AGENTS-SANDBOX-004.1: report reads cannot cross session or browser authority.
func TestSandboxReportHTTPAuthority(t *testing.T) {
	service, attempt, _ := approvalStoreFixture(t, sqlitePolicyDB(t))
	stored, err := service.CaptureReport(t.Context(), "org", attempt.ID, attempt.Version, 0, reportFixture())
	require.NoError(t, err)
	for _, test := range []struct {
		name     string
		identity authn.Identity
		session  string
		status   int
	}{
		{"member", authn.Identity{UserID: "member", OrgID: "org", Role: authn.RoleMember, SessionID: "browser"}, "session", 200},
		{"operator", authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, SessionID: "browser"}, "session", 200},
		{"PAT", authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, TokenID: "pat"}, "session", 403},
		{"foreign-org", authn.Identity{UserID: "operator", OrgID: "other", Role: authn.RoleAdmin, SessionID: "browser"}, "session", 404},
		{"foreign-session", authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, SessionID: "browser"}, "other", 404},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := requestReport(t, service, test.identity, test.session, attempt.ID)
			require.Equal(t, test.status, result.Code)
			if result.Code == http.StatusOK {
				var report Report
				require.NoError(t, json.Unmarshal(result.Body.Bytes(), &report))
				require.Equal(t, stored, &report)
				require.Equal(t, "no-store", result.Header().Get("Cache-Control"))
				require.NotContains(t, result.Body.String(), attempt.Snapshot.Launch.Root)
				require.NotContains(t, result.Body.String(), "supervisor-birth")
			}
		})
	}
}

func requestReport(t *testing.T, service *Service, identity authn.Identity, sessionID, attemptID string) *httptest.ResponseRecorder {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) { authn.SetOnGin(c, identity) })
	service.RegisterRoutes(router)
	result := httptest.NewRecorder()
	router.ServeHTTP(result, httptest.NewRequest(http.MethodGet, "/api/v1/sandbox/sessions/"+sessionID+"/attempts/"+attemptID+"/report", nil))
	return result
}

func TestSandboxReportRefreshAuthorizationAndFailure(t *testing.T) {
	service, attempt, _ := approvalStoreFixture(t, sqlitePolicyDB(t))
	identity := authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, SessionID: "browser"}
	var called int
	service.SetReportRefresher(func(ctx context.Context, orgID, attemptID string) (*Report, error) {
		called++
		require.Equal(t, "org", orgID)
		require.Equal(t, attempt.ID, attemptID)
		return service.CaptureReport(ctx, orgID, attemptID, attempt.Version, 0, reportFixture())
	})
	result := requestReport(t, service, identity, "other", attempt.ID)
	require.Equal(t, 404, result.Code)
	require.Zero(t, called)
	result = requestReport(t, service, identity, "session", attempt.ID)
	require.Equal(t, 200, result.Code)
	require.Equal(t, 1, called)
	service.SetReportRefresher(func(context.Context, string, string) (*Report, error) {
		service.SetSessionAuthorizer(func(context.Context, string) error { return ErrNotFound })
		return service.ReadCapturedReport(t.Context(), "org", attempt.ID)
	})
	result = requestReport(t, service, identity, "session", attempt.ID)
	require.Equal(t, 404, result.Code, "access revoked during provider read must hide the result")
	service.SetSessionAuthorizer(func(context.Context, string) error { return nil })
	service.SetReportRefresher(func(context.Context, string, string) (*Report, error) {
		return nil, errors.New("private runtime path and credential")
	})
	result = requestReport(t, service, identity, "session", attempt.ID)
	require.Equal(t, 500, result.Code)
	require.NotContains(t, result.Body.String(), "private")
	stored, err := service.ReadCapturedReport(t.Context(), "org", attempt.ID)
	require.NoError(t, err)
	require.Len(t, stored.Data.Records, 1, "failed refresh preserves captured evidence")
}
