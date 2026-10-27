package sandbox

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// SetReportRefresher binds the trusted runtime reader before HTTP serving starts.
func (s *Service) SetReportRefresher(refresh func(context.Context, string, string) (*Report, error)) {
	s.refreshReport = refresh
}

func (s *Service) readReportHTTP(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	ctx := c.Request.Context()
	sessionID, attemptID := c.Param("id"), c.Param("attempt")
	orgID, err := s.authorizeReport(ctx, sessionID, attemptID)
	if err == nil && s.refreshReport != nil {
		_, err = s.refreshReport(ctx, orgID, attemptID)
	}
	if err != nil {
		sandboxJSON(c, 0, nil, err)
		return
	}
	// Recheck session access after the provider call, then read committed evidence.
	report, err := s.ReadReport(ctx, sessionID, attemptID)
	sandboxJSON(c, http.StatusOK, report, err)
}
