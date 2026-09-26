package updates

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const errorResponseKey = "error"

// HandleGet returns the cached kandev_meta view of the selected channel. It
// never contacts an upstream. Read failures are logged and returned as a
// generic 500 so storage details do not cross the API boundary.
func HandleGet(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := svc.Get(c.Request.Context())
		if err != nil {
			svc.log.Error("updates: get failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{errorResponseKey: "failed to load updates"})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

type applyRequestBody struct {
	Confirm       string `json:"confirm"`
	TargetVersion string `json:"target_version"`
}

type channelRequestBody struct {
	Channel string `json:"channel"`
}

// HandleSetChannel changes the install-wide update source.
func HandleSetChannel(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !sameOriginOrNoOrigin(c.Request) {
			c.JSON(http.StatusForbidden, gin.H{errorResponseKey: "cross-origin update channel change is not allowed"})
			return
		}
		var req channelRequestBody
		if err := decodeSingleJSON(c.Request.Body, &req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{errorResponseKey: "invalid channel request"})
			return
		}
		resp, err := svc.SelectChannel(c.Request.Context(), req.Channel)
		switch {
		case errors.Is(err, ErrInvalidChannel):
			c.JSON(http.StatusBadRequest, gin.H{errorResponseKey: err.Error()})
		case errors.Is(err, ErrChannelUnsupported):
			c.JSON(http.StatusConflict, gin.H{errorResponseKey: err.Error()})
		case errors.Is(err, ErrRateLimited):
			writeRateLimited(c, svc)
		case errors.Is(err, ErrUpdateResolve):
			c.JSON(http.StatusBadGateway, gin.H{errorResponseKey: err.Error()})
		case err != nil:
			svc.log.Error("updates: set channel failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{errorResponseKey: "failed to set update channel"})
		default:
			c.JSON(http.StatusOK, resp)
		}
	}
}

// HandleCheck triggers a synchronous poll of the selected source. When the
// per-process limiter denies the request a 429 is returned with
// retry_after_seconds. Other upstream errors are surfaced as 502.
func HandleCheck(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := svc.Check(c.Request.Context())
		if errors.Is(err, ErrRateLimited) {
			writeRateLimited(c, svc)
			return
		}
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{errorResponseKey: err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

func writeRateLimited(c *gin.Context, svc *Service) {
	retry := svc.RetryAfter()
	seconds := int64(math.Ceil(retry.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	c.JSON(http.StatusTooManyRequests, gin.H{
		errorResponseKey:      ErrRateLimited.Error(),
		"retry_after_seconds": seconds,
	})
}

func decodeSingleJSON(reader io.Reader, target any) error {
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body contains multiple JSON values")
		}
		return err
	}
	return nil
}

// HandleApply queues a service-managed self-update. It is deliberately gated
// behind service metadata and a browser same-origin check because the helper
// mutates the local Kandev installation and restarts the service.
func HandleApply(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !sameOriginOrNoOrigin(c.Request) {
			c.JSON(http.StatusForbidden, gin.H{errorResponseKey: "cross-origin update apply is not allowed"})
			return
		}
		var req applyRequestBody
		if err := decodeSingleJSON(c.Request.Body, &req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{errorResponseKey: "invalid update request"})
			return
		}
		jobID, err := svc.Apply(context.Background(), req.Confirm, req.TargetVersion)
		if errors.Is(err, ErrApplyConfirm) {
			c.JSON(http.StatusBadRequest, gin.H{errorResponseKey: err.Error()})
			return
		}
		if errors.Is(err, ErrNoUpdateAvailable) || errors.Is(err, ErrUpdateTargetChanged) ||
			errors.Is(err, ErrApplyUnsupported) ||
			errors.Is(err, ErrApplyInProgress) {
			c.JSON(http.StatusConflict, gin.H{errorResponseKey: err.Error()})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{errorResponseKey: err.Error()})
			return
		}
		c.JSON(http.StatusAccepted, ApplyResponse{JobID: jobID})
	}
}
