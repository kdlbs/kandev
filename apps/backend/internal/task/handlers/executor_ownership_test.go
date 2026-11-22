package handlers

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/stretchr/testify/require"
)

func TestExecutorOwnershipHTTPDenialDoesNotDiscloseProfile(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			repo := executorFixture()
			repo.profileByID["pr-1"].OwnerUserID = "alice"
			repo.profileByID["pr-1"].Config = map[string]string{"host": "private-host"}
			h := newProfileHandlers(t, repo, nil)
			c, recorder := newWorkflowRequest(t, method, "/api/v1/executors/ex-docker/profiles/pr-1", "{}")
			c.Params = gin.Params{{Key: "id", Value: "ex-docker"}, {Key: "profileId", Value: "pr-1"}}
			c.Request = c.Request.WithContext(authn.WithIdentity(c.Request.Context(), authn.Identity{UserID: "bob", Role: authn.RoleMember}))
			switch method {
			case http.MethodGet:
				h.httpGetProfile(c)
			case http.MethodPatch:
				h.httpUpdateProfile(c)
			case http.MethodDelete:
				h.httpDeleteProfile(c)
			}
			require.Equal(t, http.StatusNotFound, recorder.Code)
			require.NotContains(t, recorder.Body.String(), "private-host")
			require.Empty(t, repo.updatedProfile)
			require.Empty(t, repo.deletedProfile)
		})
	}
}
