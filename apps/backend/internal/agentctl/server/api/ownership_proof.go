package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/ownershipproof"
)

// ProveOwnership returns one proof per credential currently acceptable to
// this server, replacement first. There are at most two, and the superseded
// one is included while its rotation is unconfirmed because an adopting
// backend that crashed mid-rotation still holds it and must be able to
// adopt again.
func (c *credentialState) ProveOwnership(challenge, binding string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	proofs := []string{ownershipproof.Derive(c.latest, challenge, binding)}
	if c.unconfirmed && c.superseded != "" {
		proofs = append(proofs, ownershipproof.Derive(c.superseded, challenge, binding))
	}
	return proofs
}

type ownershipProveRequest struct {
	Challenge string `json:"challenge"`
}

// handleOwnershipProve lets an adopting backend verify this process is the
// control server its record names before it sends the credential. It sits
// below authentication because it is what establishes that trust; answering
// it reveals nothing, since a proof is a keyed digest over a challenge the
// caller chose and cannot be inverted to the credential.
func (m *ControlServer) handleOwnershipProve(c *gin.Context) {
	if m.ownership.IsShuttingDown() {
		c.JSON(http.StatusConflict, gin.H{errKey: shuttingDownMessage})
		return
	}

	var req ownershipProveRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Challenge == "" {
		c.JSON(http.StatusBadRequest, gin.H{errKey: "missing or invalid challenge"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"proofs": m.credentials.ProveOwnership(req.Challenge, m.cfg.HomeDir)})
}

// handleOwnershipDetails carries the values an adopting backend records.
// They are filesystem paths, so they are served here, behind authentication,
// rather than on the unauthenticated identity endpoint.
func (m *ControlServer) handleOwnershipDetails(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"home_dir":            m.cfg.HomeDir,
		"diagnostic_log_path": m.cfg.DiagnosticLogPath,
	})
}
