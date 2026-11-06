package github

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (c *MockController) setRepositoryDetails(ctx *gin.Context) {
	var request struct {
		Owner       string `json:"owner" binding:"required"`
		Repo        string `json:"repo" binding:"required"`
		PushAccess  *bool  `json:"push_access"`
		AdminAccess bool   `json:"admin_access"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil {
		respondInvalidPayload(ctx)
		return
	}
	c.mock.SetRepositoryDetails(GitHubRepository{
		Owner: request.Owner, Name: request.Repo, FullName: request.Owner + "/" + request.Repo,
		PushAccessKnown: request.PushAccess != nil,
		PushAccess:      request.PushAccess != nil && *request.PushAccess,
		AdminAccess:     request.AdminAccess,
	})
	ctx.JSON(http.StatusOK, gin.H{"ok": true})
}
