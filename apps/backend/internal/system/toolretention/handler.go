package toolretention

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// HTTPService is the retention operations exposed by the settings endpoints.
type HTTPService interface {
	Get(context.Context) (Status, error)
	Save(context.Context, Update) (Status, error)
	Analyze(context.Context, Age) (string, error)
	Run(context.Context, int64) (string, error)
	Cancel(context.Context, string) (Status, error)
}

// RegisterRoutes keeps status readable while all work requires admin admission.
func RegisterRoutes(read, admin *gin.RouterGroup, service HTTPService) {
	const base = "/database/tool-payload-retention"
	read.GET(base, func(c *gin.Context) {
		status, err := service.Get(c.Request.Context())
		respond(c, http.StatusOK, status, err)
	})
	admin.PUT(base, handleSave(service))
	admin.POST(base+"/analyze", func(c *gin.Context) {
		var req struct {
			Age *Age `json:"age"`
		}
		if !decodeBody(c, &req) {
			return
		}
		if req.Age == nil {
			badRequest(c)
			return
		}
		id, err := service.Analyze(c.Request.Context(), *req.Age)
		respond(c, http.StatusAccepted, gin.H{"operation_id": id}, err)
	})
	admin.POST(base+"/run", func(c *gin.Context) {
		var req struct {
			Revision *int64 `json:"revision"`
		}
		if !decodeBody(c, &req) {
			return
		}
		if req.Revision == nil || *req.Revision < 0 {
			badRequest(c)
			return
		}
		id, err := service.Run(c.Request.Context(), *req.Revision)
		respond(c, http.StatusAccepted, gin.H{"operation_id": id}, err)
	})
	admin.POST(base+"/cancel", func(c *gin.Context) {
		var req struct {
			OperationID string `json:"operation_id"`
		}
		if !decodeBody(c, &req) {
			return
		}
		if req.OperationID == "" {
			badRequest(c)
			return
		}
		status, err := service.Cancel(c.Request.Context(), req.OperationID)
		respond(c, http.StatusOK, status, err)
	})
}

func handleSave(service HTTPService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Enabled      *bool  `json:"enabled"`
			Age          *Age   `json:"age"`
			Revision     *int64 `json:"revision"`
			BackupChoice string `json:"backup_choice,omitempty"`
		}
		if !decodeBody(c, &req) {
			return
		}
		if req.Enabled == nil || req.Age == nil || req.Revision == nil || *req.Revision < 0 {
			badRequest(c)
			return
		}
		status, err := service.Save(c.Request.Context(), Update{Enabled: *req.Enabled, Age: *req.Age, Revision: *req.Revision, BackupChoice: req.BackupChoice})
		respond(c, http.StatusOK, status, err)
	}
}

func decodeBody(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		badRequest(c)
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		badRequest(c)
		return false
	}
	return true
}

func badRequest(c *gin.Context) { c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request"}) }

func respond(c *gin.Context, status int, result any, err error) {
	if err == nil {
		c.JSON(status, result)
		return
	}
	code := err.Error()
	switch code {
	case "invalid_age", "invalid_backup_choice", "backup_choice_required":
		status = http.StatusBadRequest
	case "conflict", "busy", "preparation_required":
		status = http.StatusConflict
	case "unsupported":
		status = http.StatusNotImplemented
	default:
		status = http.StatusInternalServerError
		code = "internal_error"
	}
	c.JSON(status, gin.H{"code": code})
}
