package routines

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/office/models"
)

// Handler provides HTTP handlers for routine routes.
type Handler struct {
	svc *RoutineService
}

// NewHandler creates a new Handler backed by the given RoutineService.
func NewHandler(svc *RoutineService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers routine HTTP routes on the given router group.
func RegisterRoutes(api *gin.RouterGroup, h *Handler) {
	api.GET("/workspaces/:wsId/routines", h.listRoutines)
	api.POST("/workspaces/:wsId/routines", h.createRoutine)
	api.GET("/workspaces/:wsId/routine-runs", h.listAllRuns)
	api.GET("/routines/:id", h.getRoutine)
	api.PATCH("/routines/:id", h.updateRoutine)
	api.DELETE("/routines/:id", h.deleteRoutine)
	api.POST("/routines/:id/run", h.runRoutine)
	api.GET("/routines/:id/triggers", h.listTriggers)
	api.POST("/routines/:id/triggers", h.createTrigger)
	api.DELETE("/routine-triggers/:triggerId", h.deleteTrigger)
	api.GET("/routines/:id/runs", h.listRuns)
	api.POST("/routine-triggers/:publicId/fire", h.fireWebhookTrigger)
}

func (h *Handler) listRoutines(c *gin.Context) {
	routines, err := h.svc.ListRoutinesFromConfig(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, RoutineListResponse{Routines: routines})
}

func (h *Handler) createRoutine(c *gin.Context) {
	var req CreateRoutineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	routine := &Routine{
		WorkspaceID:            c.Param("wsId"),
		Name:                   req.Name,
		Description:            req.Description,
		TaskTemplate:           req.TaskTemplate,
		AssigneeAgentProfileID: req.AssigneeAgentProfileID,
		Status:                 "active",
		ConcurrencyPolicy:      models.RoutineConcurrencyPolicy(req.ConcurrencyPolicy),
		CatchUpPolicy:          models.RoutineCatchUpPolicy(req.CatchUpPolicy),
		CatchUpMax:             req.CatchUpMax,
		Variables:              req.Variables,
	}
	if err := h.svc.CreateRoutine(c.Request.Context(), routine); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, RoutineResponse{Routine: routine})
}

func (h *Handler) getRoutine(c *gin.Context) {
	routine, err := h.svc.GetRoutineFromConfig(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, RoutineResponse{Routine: routine})
}

func (h *Handler) updateRoutine(c *gin.Context) {
	routine, statusCode, err := h.doUpdateRoutine(c)
	if err != nil {
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, RoutineResponse{Routine: routine})
}

func (h *Handler) doUpdateRoutine(c *gin.Context) (*Routine, int, error) {
	ctx := c.Request.Context()
	routine, err := h.svc.GetRoutineFromConfig(ctx, c.Param("id"))
	if err != nil {
		return nil, http.StatusNotFound, err
	}
	var req UpdateRoutineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, http.StatusBadRequest, err
	}
	applyRoutineUpdates(routine, &req)
	if err := h.svc.UpdateRoutine(ctx, routine); err != nil {
		return nil, http.StatusInternalServerError, err
	}
	return routine, http.StatusOK, nil
}

func (h *Handler) deleteRoutine(c *gin.Context) {
	if err := h.svc.DeleteRoutine(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) runRoutine(c *gin.Context) {
	var req RunRoutineRequest
	// Body is optional for manual trigger.
	_ = c.ShouldBindJSON(&req)
	run, err := h.svc.FireManual(c.Request.Context(), c.Param("id"), req.Variables)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, RoutineRunResponse{Run: run})
}

func (h *Handler) listTriggers(c *gin.Context) {
	triggers, err := h.svc.ListRoutineTriggers(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	redactTriggerSecrets(triggers)
	c.JSON(http.StatusOK, TriggerListResponse{Triggers: triggers})
}

func (h *Handler) createTrigger(c *gin.Context) {
	var req CreateTriggerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	trigger := &RoutineTrigger{
		RoutineID:      c.Param("id"),
		Kind:           req.Kind,
		CronExpression: req.CronExpression,
		Timezone:       req.Timezone,
		PublicID:       req.PublicID,
		SigningMode:    req.SigningMode,
		Secret:         req.Secret,
		Enabled:        true,
	}
	if err := h.svc.CreateRoutineTrigger(c.Request.Context(), trigger); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	trigger.Secret = "" // redact before sending response
	c.JSON(http.StatusCreated, TriggerResponse{Trigger: trigger})
}

func (h *Handler) deleteTrigger(c *gin.Context) {
	if err := h.svc.DeleteRoutineTrigger(c.Request.Context(), c.Param("triggerId")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) listRuns(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	runs, err := h.svc.ListRoutineRuns(c.Request.Context(), c.Param("id"), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, RunListResponse{Runs: runs})
}

func (h *Handler) listAllRuns(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	runs, err := h.svc.ListAllRoutineRuns(c.Request.Context(), c.Param("wsId"), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, RunListResponse{Runs: runs})
}

func (h *Handler) fireWebhookTrigger(c *gin.Context) {
	publicID := c.Param("publicId")
	ctx := c.Request.Context()

	trigger, err := h.svc.GetTriggerByPublicID(ctx, publicID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "trigger not found"})
		return
	}
	if !trigger.Enabled {
		c.JSON(http.StatusConflict, gin.H{"error": "trigger is disabled"})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot read body"})
		return
	}

	if !verifySignature(trigger.SigningMode, trigger.Secret, c.Request, body) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "signature verification failed"})
		return
	}

	vars := parseWebhookPayload(body)

	routine, err := h.svc.GetRoutine(ctx, trigger.RoutineID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "routine not found"})
		return
	}

	run, err := h.svc.DispatchRoutineRun(ctx, routine, trigger, "webhook", vars)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"run_id": run.ID, "status": run.Status})
}

// redactTriggerSecrets clears the Secret field on each trigger to prevent
// plaintext webhook secrets from leaking in API responses.
// NOTE: Webhook secrets are stored as plaintext because HMAC verification
// requires the raw value; bcrypt-style hashing is not feasible here.
// Generation uses crypto/rand (see service layer).
func redactTriggerSecrets(triggers []*RoutineTrigger) {
	for _, t := range triggers {
		t.Secret = ""
	}
}

func applyRoutineUpdates(routine *Routine, req *UpdateRoutineRequest) {
	if req.Name != nil {
		routine.Name = *req.Name
	}
	if req.Description != nil {
		routine.Description = *req.Description
	}
	if req.TaskTemplate != nil {
		routine.TaskTemplate = *req.TaskTemplate
	}
	if req.AssigneeAgentProfileID != nil {
		routine.AssigneeAgentProfileID = *req.AssigneeAgentProfileID
	}
	if req.Status != nil {
		routine.Status = *req.Status
	}
	if req.ConcurrencyPolicy != nil {
		routine.ConcurrencyPolicy = models.RoutineConcurrencyPolicy(*req.ConcurrencyPolicy)
	}
	if req.CatchUpPolicy != nil {
		routine.CatchUpPolicy = models.RoutineCatchUpPolicy(*req.CatchUpPolicy)
	}
	if req.CatchUpMax != nil {
		routine.CatchUpMax = *req.CatchUpMax
	}
	if req.Variables != nil {
		routine.Variables = *req.Variables
	}
}

// verifySignature checks the request against the configured signing mode.
func verifySignature(mode, secret string, r *http.Request, body []byte) bool {
	switch mode {
	case "none", "":
		return true
	case "bearer":
		return verifyBearer(r, secret)
	case "hmac_sha256":
		return verifyHMAC(r, body, secret)
	default:
		return false
	}
}

func verifyBearer(r *http.Request, secret string) bool {
	auth := r.Header.Get("Authorization")
	return hmac.Equal([]byte(strings.TrimPrefix(auth, "Bearer ")), []byte(secret))
}

func verifyHMAC(r *http.Request, body []byte, secret string) bool {
	sig := r.Header.Get("X-Signature-256")
	if sig == "" {
		return false
	}
	sig = strings.TrimPrefix(sig, "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expected))
}

func parseWebhookPayload(body []byte) map[string]string {
	vars := make(map[string]string)
	if len(body) == 0 {
		return vars
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return vars
	}
	for k, v := range raw {
		switch val := v.(type) {
		case string:
			vars[k] = val
		default:
			b, _ := json.Marshal(v)
			vars[k] = string(b)
		}
	}
	return vars
}
