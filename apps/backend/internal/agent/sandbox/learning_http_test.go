package sandbox

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/stretchr/testify/require"
)

func TestSandboxLearningHTTPRequiresExplicitOperatorPublication(t *testing.T) {
	service, attempt, request := learningFixture(t, sqlitePolicyDB(t))
	operator := authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, SessionID: "browser"}
	for _, identity := range []authn.Identity{
		{UserID: "member", OrgID: "org", Role: authn.RoleMember, SessionID: "browser"},
		{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, TokenID: "token"},
	} {
		result := learningHTTP(t, service, identity, http.MethodPost, "/sessions/session/attempts/"+attempt.ID+"/learning", request)
		require.Equal(t, http.StatusForbidden, result.Code)
	}
	result := learningHTTP(t, service, operator, http.MethodPost, "/sessions/session/attempts/"+attempt.ID+"/learning", request)
	require.Equal(t, http.StatusCreated, result.Code)
	var proposal LearningProposal
	require.NoError(t, json.Unmarshal(result.Body.Bytes(), &proposal))
	_, err := service.GetLatest(operatorContext(t), proposal.Policy.ID)
	require.ErrorIs(t, err, ErrNotFound)
	result = learningHTTP(t, service, operator, http.MethodPost, "/learning/"+proposal.ID+"/publish", map[string]string{"expected_digest": "old"})
	require.Equal(t, http.StatusConflict, result.Code)
	result = learningHTTP(t, service, operator, http.MethodPost, "/learning/"+proposal.ID+"/publish", map[string]string{"expected_digest": proposal.Digest})
	require.Equal(t, http.StatusCreated, result.Code)
	var revision Revision
	require.NoError(t, json.Unmarshal(result.Body.Bytes(), &revision))
	require.Equal(t, proposal.ID, revision.LearningProposalID)
	require.NotNil(t, revision.Learning)
	result = learningHTTP(t, service, operator, http.MethodGet, "/learning/"+proposal.ID, nil)
	require.Equal(t, http.StatusOK, result.Code)
	require.Equal(t, "no-store", result.Header().Get("Cache-Control"))
	require.NoError(t, json.Unmarshal(result.Body.Bytes(), &proposal))
	require.Equal(t, revision.ID, proposal.PublishedRevisionID)
}

func learningHTTP(t *testing.T, service *Service, identity authn.Identity, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	router := gin.New()
	router.Use(func(c *gin.Context) { authn.SetOnGin(c, identity) })
	service.RegisterRoutes(router)
	result := httptest.NewRecorder()
	router.ServeHTTP(result, httptest.NewRequest(method, "/api/v1/sandbox"+path, bytes.NewReader(raw)))
	return result
}
