package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// SurvivalCapabilities lists every named capability this build of agentctl
// advertises on GET /identity. Adoption gates on the backend's required set
// being a SUBSET of this list (design 01, "Capability compatibility") -- a
// set comparison rather than a version match, so a future agentctl build can
// add capabilities without breaking older backends, and an older agentctl
// can be correctly refused by a newer backend that requires one it lacks.
var SurvivalCapabilities = []string{"agent-survival.v1"}

// handleIdentity reports installation identity and capability scope. It is
// deliberately exempt from bearer-token auth (see NewControlServer): design
// 01's "Capability compatibility" requires identity retrieval to sit below
// auth and capability negotiation, since it is what decides both.
func (m *ControlServer) handleIdentity(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"home_dir":            m.cfg.HomeDir,
		"server_identity":     m.cfg.ServerIdentity,
		"capabilities":        SurvivalCapabilities,
		"diagnostic_log_path": m.cfg.DiagnosticLogPath,
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
