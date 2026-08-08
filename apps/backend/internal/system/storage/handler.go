package storage

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type SettingsManager interface {
	GetSettings(context.Context) (StorageMaintenanceSettings, error)
	SaveSettingsWithConfirmations(context.Context, StorageMaintenanceSettings, SaveConfirmations) (StorageMaintenanceSettings, error)
}

type RunLister interface {
	ListRuns(context.Context, int) ([]MaintenanceRun, error)
}

type QuarantineLister interface {
	ListQuarantineEntries(context.Context, bool) ([]QuarantineEntry, error)
}

type Mutations interface {
	AdoptGoCache(context.Context, string, string) (StorageMaintenanceSettings, Capabilities, error)
	Analyze(context.Context) (string, error)
	RunNow(context.Context, []string, bool) (string, error)
	RestoreQuarantine(context.Context, string) (QuarantineEntry, error)
	DeleteQuarantine(context.Context, string, string) (string, error)
	PurgeQuarantine(context.Context, QuarantinePurgeScope, string) (string, error)
}

type Capabilities struct {
	ManagedGoCachePath       string `json:"managed_go_cache_path"`
	GoCacheAdoptionAvailable bool   `json:"go_cache_adoption_available"`
	DockerAvailable          bool   `json:"docker_available"`
	DockerHost               string `json:"docker_host"`
	HostGlobalDockerCleanup  bool   `json:"host_global_docker_cleanup_allowed"`
}

type Summary struct {
	Workspaces any `json:"workspaces"`
	GoCache    any `json:"go_cache"`
	Quarantine any `json:"quarantine"`
	Docker     any `json:"docker"`
}

type DiskCapacity struct {
	Path           string  `json:"path"`
	TotalBytes     uint64  `json:"total_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsedPercent    float64 `json:"used_percent"`
	Available      bool    `json:"available"`
	Warning        string  `json:"warning,omitempty"`
}

type DiskCapacityReader func(context.Context, string) (DiskCapacity, error)

func (r DiskCapacityReader) ReadDiskCapacity(ctx context.Context, path string) (DiskCapacity, error) {
	return r(ctx, path)
}

type QuarantineSummary struct {
	Count     int   `json:"count" db:"count"`
	SizeBytes int64 `json:"size_bytes" db:"size_bytes"`
}

type OverviewProvider interface {
	Summary(context.Context) (Summary, error)
	Capabilities(context.Context, StorageMaintenanceSettings) Capabilities
	SettingsCapabilities(context.Context, StorageMaintenanceSettings) Capabilities
}

type OverviewReader interface {
	Get(context.Context) (OverviewSnapshot, error)
	Capabilities(context.Context, StorageMaintenanceSettings) Capabilities
	SettingsCapabilities(context.Context, StorageMaintenanceSettings) Capabilities
}

type HandlerConfig struct {
	Settings          SettingsManager
	Runs              RunLister
	Quarantine        QuarantineLister
	Overview          OverviewReader
	DiskCapacity      DiskCapacityReader
	DiskPath          string
	Mutations         Mutations
	OnSettingsChanged func(StorageMaintenanceSettings)
	LogError          func(string, error)
}

type Handler struct {
	config HandlerConfig
}

func NewHandler(config HandlerConfig) *Handler {
	return &Handler{config: config}
}

func (h *Handler) logError(message string, err error) {
	if h.config.LogError != nil {
		h.config.LogError(message, err)
	}
}

func RegisterRoutes(group *gin.RouterGroup, handler *Handler) {
	group.GET("/storage", handler.getStorage)
	group.GET("/storage/disk", handler.getStorageDisk)
	group.GET("/storage/settings", handler.getStorageSettings)
	group.PATCH("/storage/settings", handler.patchSettings)
	group.POST("/storage/go-cache/adopt", handler.adoptGoCache)
	group.POST("/storage/analyze", handler.analyze)
	group.POST("/storage/run", handler.runNow)
	group.GET("/storage/runs", handler.listRuns)
	group.GET("/storage/quarantine", handler.listQuarantine)
	group.POST("/storage/quarantine/:id/restore", handler.restoreQuarantine)
	group.DELETE("/storage/quarantine", handler.deleteQuarantineBulk)
	group.DELETE("/storage/quarantine/:id", handler.deleteQuarantine)
}

func (h *Handler) getStorageDisk(c *gin.Context) {
	result := DiskCapacity{Path: h.config.DiskPath}
	if h.config.DiskCapacity == nil {
		result.Warning = "disk usage unavailable"
		c.JSON(http.StatusOK, result)
		return
	}
	capacity, err := h.config.DiskCapacity.ReadDiskCapacity(c.Request.Context(), h.config.DiskPath)
	if err != nil {
		h.logError("failed to read storage disk capacity", err)
		result.Warning = "disk usage unavailable"
		c.JSON(http.StatusOK, result)
		return
	}
	capacity.Path = h.config.DiskPath
	capacity.Available = true
	c.JSON(http.StatusOK, capacity)
}

func (h *Handler) listRuns(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	runs, err := h.config.Runs.ListRuns(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"runs": runs})
}

func (h *Handler) listQuarantine(c *gin.Context) {
	entries, err := h.config.Quarantine.ListQuarantineEntries(c.Request.Context(), false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"entries": entries})
}

type adoptRequest struct {
	Path    string `json:"path" binding:"required"`
	Confirm string `json:"confirm" binding:"required"`
}

func (h *Handler) adoptGoCache(c *gin.Context) {
	var request adoptRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Confirm != "ADOPT" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Go-cache adoption requires ADOPT confirmation"})
		return
	}
	settings, capabilities, err := h.config.Mutations.AdoptGoCache(c.Request.Context(), request.Path, request.Confirm)
	if err != nil {
		writeMutationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"settings": settings, "capabilities": capabilities})
}

type runRequest struct {
	Resources []string `json:"resources"`
	Force     bool     `json:"force"`
}

func (h *Handler) analyze(c *gin.Context) {
	id, err := h.config.Mutations.Analyze(c.Request.Context())
	writeAcceptedJob(c, id, err)
}

func (h *Handler) runNow(c *gin.Context) {
	var request runRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	id, err := h.config.Mutations.RunNow(c.Request.Context(), request.Resources, request.Force)
	var busy *BusyError
	if errors.As(err, &busy) {
		c.JSON(http.StatusConflict, gin.H{
			"error": busy.Error(), "busy_resources": busy.Resources,
			"force_available": busy.ForceAvailable,
		})
		return
	}
	writeAcceptedJob(c, id, err)
}

func (h *Handler) restoreQuarantine(c *gin.Context) {
	entry, err := h.config.Mutations.RestoreQuarantine(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeMutationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"entry": entry})
}

type confirmationRequest struct {
	Confirm string `json:"confirm" binding:"required"`
}

func (h *Handler) deleteQuarantine(c *gin.Context) {
	var request confirmationRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Confirm != QuarantineConfirmationDelete {
		c.JSON(http.StatusBadRequest, gin.H{"error": "quarantine deletion requires DELETE confirmation"})
		return
	}
	id, err := h.config.Mutations.DeleteQuarantine(c.Request.Context(), c.Param("id"), request.Confirm)
	writeAcceptedJob(c, id, err)
}

type bulkQuarantineRequest struct {
	Scope   QuarantinePurgeScope `json:"scope" binding:"required"`
	Confirm string               `json:"confirm" binding:"required"`
}

func (h *Handler) deleteQuarantineBulk(c *gin.Context) {
	var request bulkQuarantineRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if (request.Scope == QuarantinePurgeScopeEligible && request.Confirm != QuarantineConfirmationEligible) ||
		(request.Scope == QuarantinePurgeScopeAll && request.Confirm != QuarantineConfirmationForce) ||
		(request.Scope != QuarantinePurgeScopeEligible && request.Scope != QuarantinePurgeScopeAll) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid quarantine purge scope or confirmation"})
		return
	}
	id, err := h.config.Mutations.PurgeQuarantine(c.Request.Context(), request.Scope, request.Confirm)
	writeAcceptedJob(c, id, err)
}

func writeAcceptedJob(c *gin.Context, id string, err error) {
	if err != nil {
		writeMutationError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"job_id": id})
}

func writeMutationError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrValidation), errors.Is(err, ErrAdoptionRequired),
		errors.Is(err, ErrDedicatedDockerConfirmation):
		status = http.StatusBadRequest
	case errors.Is(err, ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, ErrConflict):
		status = http.StatusConflict
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

func (h *Handler) getStorage(c *gin.Context) {
	settings, err := h.config.Settings.GetSettings(c.Request.Context())
	if err != nil {
		h.logError("failed to load storage settings", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load storage settings"})
		return
	}
	snapshot, err := h.config.Overview.Get(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	runs, err := h.config.Runs.ListRuns(c.Request.Context(), 1)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var lastRun *MaintenanceRun
	if len(runs) > 0 {
		lastRun = &runs[0]
	}
	c.JSON(http.StatusOK, gin.H{
		"settings": settings, "capabilities": h.config.Overview.Capabilities(c.Request.Context(), settings),
		"summary": snapshot.Summary, "analyzed_at": snapshot.AnalyzedAt, "last_run": lastRun,
	})
}

func (h *Handler) getStorageSettings(c *gin.Context) {
	settings, err := h.config.Settings.GetSettings(c.Request.Context())
	if err != nil {
		h.logError("failed to load storage settings", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load storage settings"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"settings":     settings,
		"capabilities": h.config.Overview.SettingsCapabilities(c.Request.Context(), settings),
	})
}

type patchSettingsRequest struct {
	Settings      StorageMaintenanceSettings `json:"settings" binding:"required"`
	Confirmations struct {
		DedicatedDocker string `json:"dedicated_docker"`
	} `json:"confirmations"`
}

func (h *Handler) patchSettings(c *gin.Context) {
	var request patchSettingsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	saved, err := h.config.Settings.SaveSettingsWithConfirmations(
		c.Request.Context(), request.Settings,
		SaveConfirmations{DedicatedDocker: request.Confirmations.DedicatedDocker == "DEDICATED"},
	)
	if err != nil {
		if errors.Is(err, ErrValidation) || errors.Is(err, ErrAdoptionRequired) ||
			errors.Is(err, ErrDedicatedDockerConfirmation) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save storage settings"})
		return
	}
	if h.config.OnSettingsChanged != nil {
		h.config.OnSettingsChanged(saved)
	}
	c.JSON(http.StatusOK, gin.H{"settings": saved})
}
