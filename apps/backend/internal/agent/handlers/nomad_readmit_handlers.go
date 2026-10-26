package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
)

const (
	nomadReadmitStatusOperatorReleased = "operator_released"
	nomadReadmitErrorKey               = "error"
	nomadReadmitMaxBodyBytes           = 16 << 10
)

// NomadReadmitter records an operator release of one retained Nomad row.
type NomadReadmitter interface {
	ReadmitNomadEnvironment(ctx context.Context, req nomad.NomadReadmitRequest) error
}

// NomadReadmitHandlers serves the operator readmission endpoint.
type NomadReadmitHandlers struct {
	readmitter NomadReadmitter
	logger     *logger.Logger
}

type nomadReadmitBody struct {
	NodeID        string                    `json:"node_id"`
	EnvironmentID string                    `json:"environment_id"`
	Generation    int64                     `json:"generation"`
	JobID         string                    `json:"job_id"`
	Attestation   nomad.OperatorAttestation `json:"attestation"`
}

// RegisterNomadReadmitRoutes mounts the admin-only POST
// /api/v1/executors/nomad/readmit on the authenticated API router.
func RegisterNomadReadmitRoutes(router *gin.Engine, readmitter NomadReadmitter, log *logger.Logger) {
	handlers := &NomadReadmitHandlers{readmitter: readmitter, logger: log}
	router.Group("/api/v1").POST("/executors/nomad/readmit", authn.RequireAdmin(), handlers.httpReadmit)
}

func (h *NomadReadmitHandlers) httpReadmit(c *gin.Context) {
	var body nomadReadmitBody
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, nomadReadmitMaxBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		c.JSON(http.StatusBadRequest, nomadReadmitError("invalid readmission request"))
		return
	}
	err := h.readmitter.ReadmitNomadEnvironment(c.Request.Context(), nomad.NomadReadmitRequest{
		NodeID: body.NodeID, EnvironmentID: body.EnvironmentID, Generation: body.Generation,
		JobID: body.JobID, Attestation: body.Attestation,
	})
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"status": nomadReadmitStatusOperatorReleased})
	case errors.Is(err, nomad.ErrNomadReadmitInvalid):
		c.JSON(http.StatusBadRequest, nomadReadmitError("invalid readmission request"))
	case errors.Is(err, nomad.ErrNomadReadmitNotFound):
		c.JSON(http.StatusNotFound, nomadReadmitError("no matching retained environment"))
	case errors.Is(err, nomad.ErrNomadReadmitBusy):
		c.JSON(http.StatusConflict, nomadReadmitError("environment is busy with another operation; retry shortly"))
	case errors.Is(err, nomad.ErrNomadReadmitPhase):
		c.JSON(http.StatusConflict, nomadReadmitError("retained environment is not releasable"))
	default:
		h.logger.Error("nomad readmission failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, nomadReadmitError("readmission failed"))
	}
}

func nomadReadmitError(message string) gin.H { return gin.H{nomadReadmitErrorKey: message} }
