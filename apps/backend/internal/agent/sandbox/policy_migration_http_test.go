package sandbox

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/stretchr/testify/require"
)

func TestPolicyMigrationHTTPReviewsBeforePublication(t *testing.T) {
	service, err := NewService(sqlitePolicyDB(t), true, nil)
	require.NoError(t, err)
	source, err := service.Publish(operatorContext(t), policyFixture(t), 0)
	require.NoError(t, err)
	request := migrationRequestFixture(source)
	identity, _ := authn.IdentityFromContext(operatorContext(t))
	path := "/revisions/" + source.ID + "/migration"
	result := learningHTTP(t, service, identity, http.MethodPost, path+"/preview", request)
	require.Equal(t, http.StatusOK, result.Code, result.Body.String())
	require.Equal(t, "no-store", result.Header().Get("Cache-Control"))
	var preview PolicyMigrationPreview
	require.NoError(t, json.Unmarshal(result.Body.Bytes(), &preview))
	require.Equal(t, source.ID, preview.SourceRevisionID)
	body := map[string]any{"policy": request.Policy, "source_digest": request.SourceDigest,
		"expected_revision": request.ExpectedRevision, "expected_review_digest": preview.Digest}
	member, token, foreign, synthetic := identity, identity, identity, identity
	member.Role, token.TokenID, foreign.OrgID, synthetic.Synthetic = authn.RoleMember, "token", "foreign", true
	for _, caller := range []struct {
		identity authn.Identity
		status   int
	}{{member, 403}, {token, 403}, {foreign, 404}, {synthetic, 404}} {
		result = learningHTTP(t, service, caller.identity, http.MethodPost, path+"/preview", request)
		require.Equal(t, caller.status, result.Code, result.Body.String())
		result = learningHTTP(t, service, caller.identity, http.MethodPost, path+"/publish", body)
		require.Equal(t, caller.status, result.Code, result.Body.String())
	}
	body["expected_review_digest"] = "stale"
	result = learningHTTP(t, service, identity, http.MethodPost, path+"/publish", body)
	require.Equal(t, http.StatusConflict, result.Code)
	body["expected_review_digest"] = preview.Digest
	body["allow_all"] = true
	result = learningHTTP(t, service, identity, http.MethodPost, path+"/publish", body)
	require.Equal(t, http.StatusBadRequest, result.Code)
	delete(body, "allow_all")
	result = learningHTTP(t, service, identity, http.MethodPost, path+"/publish", body)
	require.Equal(t, http.StatusCreated, result.Code, result.Body.String())
	require.Equal(t, "no-store", result.Header().Get("Cache-Control"))
	var revision Revision
	require.NoError(t, json.Unmarshal(result.Body.Bytes(), &revision))
	require.Equal(t, preview.Digest, revision.Migration.ReviewDigest)
	result = learningHTTP(t, service, identity, http.MethodPost, path+"/publish", body)
	require.Equal(t, http.StatusConflict, result.Code)
}

func TestPolicyMigrationHTTPAbsentWhenDisabled(t *testing.T) {
	service, err := NewService(sqlitePolicyDB(t), false, nil)
	require.NoError(t, err)
	identity, _ := authn.IdentityFromContext(operatorContext(t))
	for _, action := range []string{"preview", "publish"} {
		result := learningHTTP(t, service, identity, http.MethodPost, "/revisions/source/migration/"+action, PolicyMigrationRequest{})
		require.Equal(t, http.StatusNotFound, result.Code)
	}
}
