package logbundle

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
)

const (
	maxCreateBodyBytes = 8 * 1024
	maxUploadBodyBytes = 1024 * 1024
)

type createRequest struct {
	Sources    []string `json:"sources"`
	SessionIDs []string `json:"session_ids"`
}

type uploadEnvelope struct {
	BrowserID       string          `json:"browser_id"`
	CaptureStreamID string          `json:"capture_stream_id"`
	ChunkIndex      int             `json:"chunk_index"`
	Done            bool            `json:"done"`
	StorageMode     string          `json:"storage_mode"`
	CaptureMetadata json.RawMessage `json:"capture_metadata"`
	Entries         json.RawMessage `json:"entries"`
}

type createResponse struct {
	JobView
	Reused bool `json:"reused"`
}

type acpSessionResponse struct {
	DiagnosticSession
	TaskTitle string `json:"task_title,omitempty"`
}

func RegisterRoutes(group *gin.RouterGroup, service *Service) {
	group.POST("/logs/bundles", handleCreate(service))
	group.GET("/logs/capabilities", handleCapabilities(service))
	group.GET("/logs/acp-sessions", handleACPSessions(service))
	group.GET("/logs/bundles/:id", handleGet(service))
	group.POST("/logs/bundles/:id/frontend", handleUpload(service))
	group.GET("/logs/bundles/:id/download", handleDownload(service))
}

func handleCreate(service *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := requestIdentity(c)
		if !ok {
			return
		}
		var request createRequest
		if err := decodeBoundedJSON(c, maxCreateBodyBytes, &request); err != nil {
			writeDecodeError(c, err)
			return
		}
		job, reused, err := service.CreateWithIdentity(
			c.Request.Context(), identity, request.Sources, request.SessionIDs,
		)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(http.StatusAccepted, createResponse{JobView: job, Reused: reused})
	}
}

func handleCapabilities(service *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := requestIdentity(c); !ok {
			return
		}
		c.JSON(http.StatusOK, service.Capabilities())
	}
}

func handleACPSessions(service *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := requestIdentity(c)
		if !ok {
			return
		}
		rows, err := service.ListACPSessions(c.Request.Context(), identity)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"sessions": acpSessionResponses(rows)})
	}
}

func acpSessionResponses(rows []DiagnosticSession) []acpSessionResponse {
	responses := make([]acpSessionResponse, 0, len(rows))
	for _, row := range rows {
		responses = append(responses, acpSessionResponse{
			DiagnosticSession: row,
			TaskTitle:         row.TaskTitle,
		})
	}
	return responses
}

func handleGet(service *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := requestIdentity(c)
		if !ok {
			return
		}
		job, err := service.Get(identity.UserID, c.Param("id"))
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(http.StatusOK, job)
	}
}

func handleUpload(service *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := requestIdentity(c)
		if !ok {
			return
		}
		var envelope uploadEnvelope
		if err := decodeBoundedJSON(c, maxUploadBodyBytes, &envelope); err != nil {
			writeDecodeError(c, err)
			return
		}
		ignored, claimed, err := service.ClaimStream(
			identity.UserID, c.Param("id"), envelope.BrowserID,
			envelope.CaptureStreamID, envelope.StorageMode,
		)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		if ignored {
			c.Status(http.StatusNoContent)
			return
		}
		var entries []json.RawMessage
		if len(envelope.Entries) == 0 {
			entries = []json.RawMessage{}
		} else if err := json.Unmarshal(envelope.Entries, &entries); err != nil {
			if claimed {
				service.ReleaseEmptyClaim(identity.UserID, c.Param("id"),
					envelope.BrowserID, envelope.CaptureStreamID)
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "entries must be a JSON array"})
			return
		}
		_, err = service.UploadChunk(identity.UserID, c.Param("id"), UploadChunk{
			BrowserID: envelope.BrowserID, CaptureStreamID: envelope.CaptureStreamID,
			ChunkIndex: envelope.ChunkIndex, Done: envelope.Done,
			StorageMode: envelope.StorageMode, CaptureMetadata: envelope.CaptureMetadata,
			Entries: entries,
		})
		if err != nil {
			if claimed {
				service.ReleaseEmptyClaim(identity.UserID, c.Param("id"),
					envelope.BrowserID, envelope.CaptureStreamID)
			}
			writeServiceError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func handleDownload(service *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := requestIdentity(c)
		if !ok {
			return
		}
		file, _, err := service.OpenArchive(identity.UserID, c.Param("id"))
		if err != nil {
			writeServiceError(c, err)
			return
		}
		defer func() { _ = file.Close() }()
		info, err := file.Stat()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to inspect diagnostic bundle"})
			return
		}
		c.Header("Content-Disposition", `attachment; filename="kandev-diagnostic-logs.zip"`)
		c.Header("Content-Type", "application/zip")
		http.ServeContent(c.Writer, c.Request, "kandev-diagnostic-logs.zip", info.ModTime(), file)
	}
}

func requestIdentity(c *gin.Context) (authn.Identity, bool) {
	identity, ok := authn.FromGin(c)
	if !ok || identity.UserID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return authn.Identity{}, false
	}
	return identity, true
}

func decodeBoundedJSON(c *gin.Context, limit int64, destination any) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain one JSON object")
		}
		return err
	}
	return nil
}

func writeDecodeError(c *gin.Context, err error) {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
}

func writeServiceError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	switch {
	case IsKind(err, ErrorInvalid):
		status = http.StatusBadRequest
	case IsKind(err, ErrorNotFound):
		status = http.StatusNotFound
	case IsKind(err, ErrorGone):
		status = http.StatusGone
	case IsKind(err, ErrorConflict):
		status = http.StatusConflict
	case IsKind(err, ErrorIdentityBusy), IsKind(err, ErrorProfileLimit):
		status = http.StatusTooManyRequests
		c.Header("Retry-After", strconv.Itoa(5))
	case IsKind(err, ErrorSaturated):
		status = http.StatusServiceUnavailable
		c.Header("Retry-After", strconv.Itoa(5))
	case IsKind(err, ErrorTooLarge):
		status = http.StatusRequestEntityTooLarge
	}
	c.JSON(status, gin.H{"error": err.Error()})
}
