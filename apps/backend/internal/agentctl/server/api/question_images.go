package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/imagepreview"
)

func (s *Server) handleQuestionImage(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	content, err := s.procMgr.ReadQuestionImage(ctx, c.Query("path"), c.Query("repo"))
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	if err != nil {
		code := imagepreview.Unavailable
		_ = errors.As(err, &code)
		c.JSON(code.Status(), gin.H{"code": code})
		return
	}
	c.Header("Content-Disposition", "inline")
	c.Data(http.StatusOK, content.MIME, content.Bytes)
}
