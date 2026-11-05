package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/stretchr/testify/require"
)

func TestAuditHTTPVerificationAdmissionAndReadback(t *testing.T) {
	service, attempt, _ := approvalStoreFixture(t, sqlitePolicyDB(t), 2)
	ctx := operatorContext(t)
	identity, _ := authn.IdentityFromContext(ctx)
	attempt, err := service.TransitionAttempt(ctx, "org", attempt.ID, attempt.Version, AttemptStopped, nil)
	require.NoError(t, err)
	binding, err := AuditBindingDigest(attempt)
	require.NoError(t, err)
	report, err := service.CaptureReport(ctx, "org", attempt.ID, attempt.Version, 0, ReportData{Status: "unavailable", Integrity: "unverified", Finalized: true, FinalAudit: &FinalAuditReceipt{BindingDigest: binding, ArtifactSetDigest: strings.Repeat("d", 64), CapturedAt: time.Now().UTC()}})
	require.NoError(t, err)
	request := AuditVerificationRequest{RequestID: uuid.NewString(), ExpectedReportDigest: report.Digest, ExpectedArtifactDigest: report.Data.FinalAudit.ArtifactSetDigest}
	body, err := json.Marshal(request)
	require.NoError(t, err)
	result := auditHTTPRequest(service, identity, http.MethodPost, "session", attempt.ID, "verifications", body)
	require.Equal(t, 422, result.Code, "verification requires an initialized runtime")
	calls := 0
	service.SetAuditVerifier(func(ctx context.Context, session, id string, request AuditVerificationRequest) (*AuditVerification, error) {
		calls++
		return service.BeginAuditVerification(ctx, session, id, request, auditVerificationTestEpoch)
	})
	member := authn.Identity{UserID: "member", OrgID: "org", Role: authn.RoleMember, SessionID: "browser"}
	for _, test := range []struct {
		identity authn.Identity
		session  string
		status   int
	}{{member, "session", 403}, {identity, "foreign", 404}, {authn.Identity{UserID: "operator", OrgID: "other", Role: authn.RoleAdmin, SessionID: "browser"}, "session", 404}} {
		result = auditHTTPRequest(service, test.identity, http.MethodPost, test.session, attempt.ID, "verifications", body)
		require.Equal(t, test.status, result.Code, result.Body.String())
	}
	require.Zero(t, calls, "authority must be checked before dispatch")
	result = auditHTTPRequest(service, identity, http.MethodPost, "session", attempt.ID, "verifications", body)
	require.Equal(t, 202, result.Code, result.Body.String())
	var accepted AuditVerificationView
	require.NoError(t, json.Unmarshal(result.Body.Bytes(), &accepted))
	require.Equal(t, "pending", accepted.Verification.State)
	require.Equal(t, 1, calls)
	for _, path := range []string{"verifications/" + accepted.Verification.ID, "verifications"} {
		result = auditHTTPRequest(service, member, http.MethodGet, "session", attempt.ID, path, nil)
		require.Equal(t, 200, result.Code, result.Body.String())
		require.Equal(t, "no-store", result.Header().Get("Cache-Control"))
		require.NotContains(t, result.Body.String(), auditVerificationTestEpoch)
		require.NotContains(t, result.Body.String(), attempt.Snapshot.Launch.Root)
	}
	require.Equal(t, 1, calls, "reading history must not execute verification")
	for _, privateFailure := range []error{nil, errors.New("private runtime path and credential")} {
		service.SetAuditVerifier(func(context.Context, string, string, AuditVerificationRequest) (*AuditVerification, error) {
			return nil, privateFailure
		})
		result = auditHTTPRequest(service, identity, http.MethodPost, "session", attempt.ID, "verifications", body)
		if privateFailure == nil {
			require.Equal(t, 409, result.Code)
		} else {
			require.Equal(t, 500, result.Code)
		}
		require.NotContains(t, result.Body.String(), "private")
	}
	service.SetAuditVerifier(func(ctx context.Context, session, id string, request AuditVerificationRequest) (*AuditVerification, error) {
		job, err := service.BeginAuditVerification(ctx, session, id, request, auditVerificationTestEpoch)
		service.SetSessionAuthorizer(func(context.Context, string) error { return ErrNotFound })
		return job, err
	})
	result = auditHTTPRequest(service, identity, http.MethodPost, "session", attempt.ID, "verifications", body)
	require.Equal(t, 404, result.Code, "access must be rechecked after dispatch")
}

func TestAuditHTTPRoutesAreAbsentWhenDisabled(t *testing.T) {
	service, err := NewService(sqlitePolicyDB(t), false, nil)
	require.NoError(t, err)
	identity, _ := authn.IdentityFromContext(operatorContext(t))
	for _, request := range []struct{ method, path string }{{http.MethodGet, "trust"}, {http.MethodPut, "trust"}, {http.MethodGet, "verifications"}, {http.MethodPost, "verifications"}, {http.MethodGet, "verifications/anything"}} {
		result := auditHTTPRequest(service, identity, request.method, "session", "attempt", request.path, nil)
		require.Equal(t, 404, result.Code)
	}
}

func TestAuditHTTPTrustRequiresOperatorAndExactSession(t *testing.T) {
	service, attempt, _ := approvalStoreFixture(t, sqlitePolicyDB(t), 2)
	operator, _ := authn.IdentityFromContext(operatorContext(t))
	for _, test := range []struct {
		name, method, session, body string
		identity                    authn.Identity
		status                      int
	}{
		{"read", http.MethodGet, "session", "", operator, 200},
		{"foreign-session", http.MethodGet, "foreign", "", operator, 404},
		{"member-write", http.MethodPut, "session", `{"revoke":true}`, authn.Identity{UserID: "member", OrgID: "org", Role: authn.RoleMember, SessionID: "browser"}, 403},
		{"token", http.MethodGet, "session", "", authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, TokenID: "pat"}, 403},
		{"unknown-input", http.MethodPut, "session", `{"revoke":true,"private_key":"forbidden"}`, operator, 400},
		{"write", http.MethodPut, "session", `{"revoke":true}`, operator, 200},
		{"stale-write", http.MethodPut, "session", `{"revoke":true}`, operator, 409},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := auditHTTPRequest(service, test.identity, test.method, test.session, attempt.ID, "trust", []byte(test.body))
			require.Equal(t, test.status, result.Code, result.Body.String())
			if result.Code == 200 {
				require.Equal(t, "no-store", result.Header().Get("Cache-Control"))
				var revision AuditTrustRevision
				require.NoError(t, json.Unmarshal(result.Body.Bytes(), &revision))
				require.True(t, revision.Revoked)
			}
		})
	}
	service.SetSessionAuthorizer(func(context.Context, string) error { return ErrNotFound })
	result := auditHTTPRequest(service, operator, http.MethodPut, "session", attempt.ID, "trust", []byte(`{"expected_version":1,"revoke":true}`))
	require.Equal(t, 404, result.Code)
}

func auditHTTPRequest(service *Service, identity authn.Identity, method, session, attempt, path string, body []byte) *httptest.ResponseRecorder {
	router := gin.New()
	router.Use(func(c *gin.Context) { authn.SetOnGin(c, identity) })
	service.RegisterRoutes(router)
	result := httptest.NewRecorder()
	router.ServeHTTP(result, httptest.NewRequest(method, "/api/v1/sandbox/sessions/"+session+"/attempts/"+attempt+"/audit/"+path, bytes.NewReader(body)))
	return result
}
