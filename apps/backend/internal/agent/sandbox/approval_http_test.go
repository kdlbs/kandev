package sandbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/stretchr/testify/require"
)

func TestSandboxApprovalHTTPAuthorityAndProjection(t *testing.T) {
	service, attempt, file := approvalStoreFixture(t, sqlitePolicyDB(t))
	service.SetApprovalGuard(func(context.Context, *Attempt, *Approval) error { return nil })
	approval, err := service.RecordApproval(t.Context(), "org", attempt.ID, ApprovalOperation{RequestID: "request", ProviderSessionID: "provider-session", ChildPID: 101, Access: "read", Target: file}, strings.Repeat("a", 64), time.Now().Add(30*time.Second))
	require.NoError(t, err)
	for _, test := range []struct {
		name                       string
		identity                   authn.Identity
		listStatus, decisionStatus int
	}{
		{"member", authn.Identity{UserID: "member", OrgID: "org", Role: authn.RoleMember, SessionID: "browser"}, 200, 403},
		{"PAT", authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, TokenID: "pat"}, 403, 403},
		{"other-org", authn.Identity{UserID: "operator", OrgID: "foreign", Role: authn.RoleAdmin, SessionID: "browser"}, 404, 404},
		{"operator", authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, SessionID: "browser"}, 200, 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) { authn.SetOnGin(c, test.identity) })
			service.RegisterRoutes(router)
			result := httptest.NewRecorder()
			router.ServeHTTP(result, httptest.NewRequest(http.MethodGet, "/api/v1/sandbox/sessions/session/approvals", nil))
			require.Equal(t, test.listStatus, result.Code)
			if result.Code == http.StatusOK {
				var values []ApprovalView
				require.NoError(t, json.Unmarshal(result.Body.Bytes(), &values))
				if test.identity.OrgID == "foreign" {
					require.Empty(t, values)
				} else {
					require.Len(t, values, 1)
					require.Equal(t, test.name == "operator", values[0].CanDecide)
					require.NotContains(t, result.Body.String(), "supervisor-birth")
					require.NotContains(t, result.Body.String(), "provider-session")
					require.NotContains(t, result.Body.String(), attempt.Snapshot.Launch.Root)
				}
			}
			result = httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/v1/sandbox/attempts/"+attempt.ID+"/approvals/request/decision", strings.NewReader(`{"expected_version":1,"decision":"deny"}`))
			router.ServeHTTP(result, request)
			require.Equal(t, test.decisionStatus, result.Code)
		})
	}
	final, err := service.ReadApproval(t.Context(), "org", attempt.ID, "request")
	require.NoError(t, err)
	require.Equal(t, ApprovalDenied, final.State)
	require.Equal(t, approval.Snapshot, final.Snapshot)
}

func TestSandboxApprovalHTTPRequiresCurrentSessionAccess(t *testing.T) {
	service, attempt, file := approvalStoreFixture(t, sqlitePolicyDB(t))
	service.SetApprovalGuard(func(context.Context, *Attempt, *Approval) error { return nil })
	approval, err := service.RecordApproval(t.Context(), "org", attempt.ID, ApprovalOperation{RequestID: "request", ProviderSessionID: "provider-session", ChildPID: 101, Access: "read", Target: file}, strings.Repeat("a", 64), time.Now().Add(30*time.Second))
	require.NoError(t, err)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, SessionID: "browser"})
	})
	service.RegisterRoutes(router)
	for _, kind := range []string{"missing", "denied", "allowed"} {
		permitted := kind == "allowed"
		service.SetSessionAuthorizer(func(_ context.Context, sessionID string) error {
			require.Equal(t, "session", sessionID)
			if !permitted {
				return ErrNotFound
			}
			return nil
		})
		if kind == "missing" {
			service.SetSessionAuthorizer(nil)
		}
		want := http.StatusNotFound
		if permitted {
			want = http.StatusOK
		}
		result := httptest.NewRecorder()
		router.ServeHTTP(result, httptest.NewRequest(http.MethodGet, "/api/v1/sandbox/sessions/session/approvals", nil))
		require.Equal(t, want, result.Code)
		result = httptest.NewRecorder()
		router.ServeHTTP(result, httptest.NewRequest(http.MethodPost, "/api/v1/sandbox/attempts/"+attempt.ID+"/approvals/request/decision", strings.NewReader(`{"expected_version":1,"decision":"deny"}`)))
		require.Equal(t, want, result.Code)
		if !permitted {
			stored, readErr := service.ReadApproval(t.Context(), "org", attempt.ID, "request")
			require.NoError(t, readErr)
			require.Equal(t, approval, stored)
		}
	}
}
