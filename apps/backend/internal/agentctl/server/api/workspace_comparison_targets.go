package api

import (
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// SetComparisonTargetsRequest replaces the provider-qualified comparison
// target map. Keys are workspace repository subpaths; the empty key addresses
// the root/single-repository tracker.
type SetComparisonTargetsRequest struct {
	ComparisonTargets map[string]models.ComparisonTarget `json:"comparison_targets"`
}

func (s *Server) handleSetComparisonTargets(c *gin.Context) {
	var req SetComparisonTargetsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{errKey: "invalid JSON body"})
		return
	}
	for repositoryName, target := range req.ComparisonTargets {
		if !validComparisonTargetRepositoryKey(repositoryName) {
			c.JSON(http.StatusBadRequest, gin.H{errKey: "invalid comparison target repository key"})
			return
		}
		if err := target.Validate(); err != nil {
			s.logger.Warn("comparison target request rejected", zap.String("repository", repositoryName), zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{errKey: "invalid comparison target"})
			return
		}
	}
	s.procMgr.UpdateComparisonTargets(c.Request.Context(), req.ComparisonTargets)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func validComparisonTargetRepositoryKey(repositoryName string) bool {
	if repositoryName == "" || repositoryName == "." {
		return true
	}
	if strings.TrimSpace(repositoryName) != repositoryName || strings.Contains(repositoryName, "\\") {
		return false
	}
	cleaned := path.Clean(repositoryName)
	return cleaned == repositoryName && cleaned != ".." && !strings.HasPrefix(cleaned, "../") && !strings.HasPrefix(cleaned, "/")
}
