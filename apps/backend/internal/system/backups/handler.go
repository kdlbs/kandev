package backups

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes wires the backups endpoints onto g (typically the
// /api/v1/system group).
func RegisterRoutes(g *gin.RouterGroup, svc *Service) {
	g.GET("/backups", HandleList(svc))
	g.POST("/backups", HandleCreate(svc))
	g.GET("/backups/:name/download", HandleDownload(svc))
	g.POST("/backups/:name/restore", HandleRestore(svc))
	g.DELETE("/backups/:name", HandleDelete(svc))
}

// HandleList returns GET /api/v1/system/backups -> { snapshots: [...] }.
func HandleList(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		snaps, err := svc.List()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"snapshots": snaps})
	}
}

// HandleCreate returns POST /api/v1/system/backups -> 202 { job_id }.
func HandleCreate(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := svc.Create(c.Request.Context())
		c.JSON(http.StatusAccepted, gin.H{"job_id": id})
	}
}

// HandleDownload returns GET /api/v1/system/backups/:name/download. Streams
// the file as an attachment.
func HandleDownload(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		if _, err := svc.resolveSnapshotPath(name); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid name"})
			return
		}
		f, size, err := svc.OpenForDownload(name)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		defer func() { _ = f.Close() }()
		c.Header("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(name, `"`, `\"`)+`"`)
		c.Header("Content-Type", "application/octet-stream")
		c.DataFromReader(http.StatusOK, size, "application/octet-stream", f, nil)
	}
}

// HandleRestore returns POST /api/v1/system/backups/:name/restore. Body
// must include {"confirm":"RESTORE"}.
func HandleRestore(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		if _, err := svc.resolveSnapshotPath(name); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid name"})
			return
		}
		var body struct {
			Confirm string `json:"confirm"`
		}
		_ = c.ShouldBindJSON(&body)
		id, err := svc.Restore(c.Request.Context(), name, body.Confirm)
		if err != nil {
			if errors.Is(err, errRestoreConfirm) {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"job_id": id})
	}
}

// HandleDelete returns DELETE /api/v1/system/backups/:name -> 204.
func HandleDelete(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		if _, err := svc.resolveSnapshotPath(name); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid name"})
			return
		}
		if err := svc.Delete(name); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}
