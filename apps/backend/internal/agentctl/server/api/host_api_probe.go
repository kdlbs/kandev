package api

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

const (
	hostAPIProbeTimeout  = 10 * time.Second
	maxHostAPIProbeBytes = 2048
)

// handleHostAPIProbe requests the host's ping route on behalf of the backend
// and reports the status it answered with. It never follows a redirect and
// refuses every URL that is not the ping route, so it cannot be used to
// reach another service from the worker.
func (m *ControlServer) handleHostAPIProbe(c *gin.Context) {
	// Control auth permits unauthenticated local use when no credential was
	// configured. The probe makes an outbound request, so it requires one.
	if m.credentials.Latest() == "" {
		c.JSON(http.StatusUnauthorized, gin.H{errKey: "host API probe requires control credentials"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxHostAPIProbeBytes)
	var req agentctltypes.HostAPIProbeRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Token == "" || !isHostAPIPingURL(req.URL) {
		c.JSON(http.StatusBadRequest, gin.H{errKey: "invalid host API probe request"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), hostAPIProbeTimeout)
	defer cancel()
	probe, err := http.NewRequestWithContext(ctx, http.MethodGet, req.URL, http.NoBody)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{errKey: "invalid host API probe request"})
		return
	}
	probe.Header.Set(agentctltypes.HostAPIProbeTokenHeader, req.Token)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(probe)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{errKey: "host API is unreachable"})
		return
	}
	_ = resp.Body.Close()
	c.JSON(http.StatusOK, agentctltypes.HostAPIProbeResponse{StatusCode: resp.StatusCode})
}

func isHostAPIPingURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.EscapedPath() == agentctltypes.HostAPIPingPath
}
