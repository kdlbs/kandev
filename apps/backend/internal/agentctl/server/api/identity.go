package api

import (
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/launchwrapper"
)

// SurvivalCapabilities lists every named capability this build of agentctl
// advertises on GET /identity. Adoption gates on the backend's required set
// being a SUBSET of this list (design 01, "Capability compatibility") -- a
// set comparison rather than a version match, so a future agentctl build can
// add capabilities without breaking older backends, and an older agentctl
// can be correctly refused by a newer backend that requires one it lacks.
var SurvivalCapabilities = []string{"agent-survival.v1", "agent-delivery.v1", launchwrapper.Capability, agentctltypes.RuntimeDrainVersion}

// handleIdentity reports this launch's opaque identity and capability
// scope. It is deliberately exempt from bearer-token auth (see
// NewControlServer): identity retrieval decides adoption compatibility, so
// it cannot itself be gated on the answer. Because it sits below
// authentication it carries nothing beyond what that decision needs, and in
// particular no filesystem path; adoption reads those from
// /api/v1/ownership/details once authenticated.
func (m *ControlServer) handleIdentity(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"server_identity": m.cfg.ServerIdentity,
		"capabilities":    capabilitiesFor(m.cfg.NodeLaunchWrapper),
		// unowned_period_ms is this server's own resolved unowned period
		// (AC-EXECUTORS-CONTROL-OWNERSHIP-003.2/.7), not the caller's config:
		// an adopting backend's local config can disagree with what this
		// process actually enforces (e.g. an operator raised
		// agentctl.unownedPeriod between restarts), and that mismatch drives
		// the adopting backend to renew on a cadence too slow for this
		// server's own reaper. This is the only channel that value crosses
		// back to an adopting backend.
		"unowned_period_ms": m.unownedPeriod.Milliseconds(),
	})
}

func (s *Server) handleInstanceIdentity(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"capabilities": capabilitiesFor(s.cfg.NodeLaunchWrapper)})
}

// capabilitiesFor adds the node wrapper capability only while one is loaded.
// It reveals that the wrapper is applied, never its path or contents.
func capabilitiesFor(node launchwrapper.Node) []string {
	if node.Spec.IsZero() {
		return SurvivalCapabilities
	}
	return append(slices.Clone(SurvivalCapabilities), launchwrapper.NodeCapability)
}
