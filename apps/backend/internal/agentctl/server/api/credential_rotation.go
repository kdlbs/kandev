package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

// errRotationCredentialRejected is returned when Rotate is presented a
// credential outside the acceptable set of
// AC-EXECUTORS-CONTROL-OWNERSHIP-002.8.
var errRotationCredentialRejected = errors.New("credential not accepted for rotation")

// errRotationIDNotIssued is returned when Confirm names a rotation
// identifier the control server never allocated.
var errRotationIDNotIssued = errors.New("rotation identifier not issued")

// credentialState is the two-phase credential rotation machinery of design
// 01's "Single driver" section. It starts holding a single fully-
// authenticating credential (the bootstrap token) and no superseded
// credential -- the state a freshly spawned server is in before any
// adoption has ever rotated it.
//
// A rotation replaces the fully-authenticating credential and demotes the
// previous one to "adoption-only" (it authenticates only a further rotation
// attempt or the ownership-shutdown operation, AC-002.7/AC-002.9) until the
// adopting backend confirms it has durably stored the replacement, at which
// point the superseded credential is dropped entirely.
type credentialState struct {
	mu          sync.Mutex
	rotationID  int64
	latest      string
	superseded  string
	unconfirmed bool
}

// newCredentialState seeds the machinery with the control server's initial
// (bootstrap-minted) credential. No rotation has happened yet, so there is
// no superseded credential and nothing to confirm.
func newCredentialState(initial string) *credentialState {
	return &credentialState{latest: initial}
}

// Latest returns the credential that authenticates every operation.
func (c *credentialState) Latest() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.latest
}

// AcceptsFull reports whether token authenticates a normal instance
// operation or stream (AC-EXECUTORS-CONTROL-OWNERSHIP-002.6): only the
// credential issued by the current highest-numbered rotation qualifies.
func (c *credentialState) AcceptsFull(token string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return tokenEqual(token, c.latest)
}

// AcceptsAdoptionOnly reports whether token authenticates a further
// rotation attempt or the ownership-shutdown operation
// (AC-EXECUTORS-CONTROL-OWNERSHIP-002.7, -002.9): the latest credential
// always qualifies, and the superseded one qualifies only while its
// rotation remains unconfirmed.
func (c *credentialState) AcceptsAdoptionOnly(token string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if tokenEqual(token, c.latest) {
		return true
	}
	return c.unconfirmed && c.superseded != "" && tokenEqual(token, c.superseded)
}

// Rotate replaces the latest credential. presented must be the current
// latest credential (an ordinary rotation) or, while the previous rotation
// remains unconfirmed, the credential it superseded -- which is an
// idempotent retry (AC-EXECUTORS-CONTROL-OWNERSHIP-002.10) that allocates
// no new rotation and returns the same identifier and replacement again.
func (c *credentialState) Rotate(presented string) (rotationID int64, replacement string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.unconfirmed && c.superseded != "" && tokenEqual(presented, c.superseded) {
		return c.rotationID, c.latest, nil
	}
	if !tokenEqual(presented, c.latest) {
		return 0, "", errRotationCredentialRejected
	}

	replacement = generateRotationCredential()
	c.superseded = c.latest
	c.latest = replacement
	c.rotationID++
	c.unconfirmed = true
	return c.rotationID, c.latest, nil
}

// Confirm names the rotation the caller has durably stored (both the
// replacement credential and the control-server record's reference to it).
// Naming the current highest-numbered rotation -- for the first time or
// repeated -- drops the superseded credential and is accepted. Naming any
// earlier identifier is accepted and has no effect, so a delayed or
// duplicated confirmation can never revoke a credential issued after it.
// Naming an identifier never issued is rejected.
func (c *credentialState) Confirm(rotationID int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if rotationID <= 0 || rotationID > c.rotationID {
		return errRotationIDNotIssued
	}
	if rotationID < c.rotationID {
		return nil
	}
	c.unconfirmed = false
	c.superseded = ""
	return nil
}

func generateRotationCredential() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// controlCredentialAuth authenticates control-server requests against the
// rotating credential set in creds, distinguishing adoption-only paths
// (where the superseded credential remains valid until confirmed, per
// AC-002.7/-002.9) from every other path (where only the current
// highest-numbered rotation's credential is valid). Mirrors
// bearerTokenAuth's "no credential configured disables auth" behavior when
// creds started out empty.
func controlCredentialAuth(creds *credentialState, adoptionOnlyPaths map[string]bool, exemptPaths ...string) gin.HandlerFunc {
	if creds.Latest() == "" {
		return func(c *gin.Context) { c.Next() }
	}

	exempt := make(map[string]bool, len(exemptPaths))
	for _, p := range exemptPaths {
		exempt[p] = true
	}

	return func(c *gin.Context) {
		if exempt[c.Request.URL.Path] {
			c.Next()
			return
		}

		token := extractBearerToken(c.Request)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{errKey: "missing or invalid Authorization header"})
			return
		}

		var ok bool
		if adoptionOnlyPaths[c.Request.URL.Path] {
			ok = creds.AcceptsAdoptionOnly(token)
		} else {
			ok = creds.AcceptsFull(token)
		}
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{errKey: "invalid auth token"})
			return
		}

		c.Next()
	}
}

type confirmRotationRequest struct {
	RotationID int64 `json:"rotation_id"`
}

// handleCredentialRotate is the rotate half of the two-phase rotation.
// Refused once the one-way unowned-shutdown door has fired (AC-003.9): a
// rotation is the first step of an adoption attempt, and a decided
// shutdown must never be reopened.
func (m *ControlServer) handleCredentialRotate(c *gin.Context) {
	if m.ownership.IsShuttingDown() {
		c.JSON(http.StatusConflict, gin.H{errKey: shuttingDownMessage})
		return
	}

	presented := extractBearerToken(c.Request)
	rotationID, replacement, err := m.credentials.Rotate(presented)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{errKey: err.Error()})
		return
	}

	// A successful rotation -- fresh or an idempotent replay -- renews
	// ownership: it is the first authenticated operation an adopting
	// backend issues (design 01, "Unowned shutdown").
	m.ownership.Renew()

	c.JSON(http.StatusOK, gin.H{
		"rotation_id": rotationID,
		"credential":  replacement,
	})
}

// handleCredentialConfirm is the confirm half of the two-phase rotation.
// It does not itself renew ownership: only the handshake, the claim
// operation, and rotation do (AC-003.8).
func (m *ControlServer) handleCredentialConfirm(c *gin.Context) {
	if m.ownership.IsShuttingDown() {
		c.JSON(http.StatusConflict, gin.H{errKey: shuttingDownMessage})
		return
	}

	var req confirmRotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{errKey: "missing or invalid rotation_id"})
		return
	}

	if err := m.credentials.Confirm(req.RotationID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{errKey: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{lspStatusKey: "confirmed"})
}
