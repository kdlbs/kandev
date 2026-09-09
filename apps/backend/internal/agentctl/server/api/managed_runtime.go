package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/managedruntime"
)

// ManagedRuntimeCacheRepairRequest contains the trusted exact package spec
// used to derive one npm execution tree.
type ManagedRuntimeCacheRepairRequest struct {
	PackageSpec string `json:"package_spec"`
}

// ManagedRuntimeCacheRepairResponse reports the result without exposing the
// executor's npm path or command output.
type ManagedRuntimeCacheRepairResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

func (s *Server) handleManagedRuntimeCacheRepair(c *gin.Context) {
	var req ManagedRuntimeCacheRepairRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ManagedRuntimeCacheRepairResponse{
			Error: "invalid managed runtime cache repair request",
		})
		return
	}
	if err := managedruntime.ValidateExactPackageSpec(req.PackageSpec); err != nil {
		c.JSON(http.StatusBadRequest, ManagedRuntimeCacheRepairResponse{
			Error: "invalid managed runtime package specification",
		})
		return
	}
	if err := s.procMgr.RepairManagedRuntimeCache(c.Request.Context(), req.PackageSpec); err != nil {
		if c.Request.Context().Err() != nil {
			return
		}
		s.logger.Error("managed runtime cache repair failed")
		c.JSON(http.StatusInternalServerError, ManagedRuntimeCacheRepairResponse{
			Error: "managed runtime cache repair failed",
		})
		return
	}
	c.JSON(http.StatusOK, ManagedRuntimeCacheRepairResponse{Success: true})
}
