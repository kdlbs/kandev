package coordinator

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// registerMergeRoutes mounts the merge stewardship routes.
func registerMergeRoutes(workspace *gin.RouterGroup, h *Handlers) {
	workspace.GET("/coordinators/:cid/tasks/:tid/dependencies", h.httpGetDependencies)
	workspace.PUT("/coordinators/:cid/tasks/:tid/dependencies", h.httpPutDependencies)
	workspace.POST("/coordinators/:cid/merge/protection/refresh", h.httpRefreshProtection)
	workspace.POST("/coordinators/:cid/proposals/:pid/merge/check", h.httpCheckMerge)
	workspace.POST("/coordinators/:cid/proposals/:pid/merge/release", h.httpReleaseMerge)
}

// httpCheckMerge backs POST .../proposals/:pid/merge/check (Check now).
func (h *Handlers) httpCheckMerge(c *gin.Context) {
	p, err := h.service.CheckMergeNow(c.Request.Context(), c.Param("id"), c.Param("cid"), c.Param("pid"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, NewProposalDTOPhases(p, h.service.phase2, h.service.phase3))
}

// httpReleaseMerge backs POST .../proposals/:pid/merge/release.
func (h *Handlers) httpReleaseMerge(c *gin.Context) {
	p, err := h.service.ReleaseMerge(c.Request.Context(), c.Param("id"), c.Param("cid"), c.Param("pid"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, NewProposalDTOPhases(p, h.service.phase2, h.service.phase3))
}

// httpRefreshProtection backs POST .../coordinators/:cid/merge/protection/refresh:
// it re-reads the protections of every branch shown in settings.
func (h *Handlers) httpRefreshProtection(c *gin.Context) {
	obs, err := h.service.RefreshProtectionAs(c.Request.Context(), c.Param("id"), c.Param("cid"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	if obs == nil {
		obs = []ProtectionObservation{}
	}
	c.JSON(http.StatusOK, gin.H{"protection": obs})
}

// httpGetDependencies backs GET .../coordinators/:cid/tasks/:tid/dependencies.
func (h *Handlers) httpGetDependencies(c *gin.Context) {
	set, err := h.service.GetDependencies(c.Request.Context(), c.Param("id"), c.Param("cid"), c.Param("tid"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, set)
}

// httpPutDependencies backs PUT .../coordinators/:cid/tasks/:tid/dependencies.
// The body is {"dependency_revision": n, "refs": [{"task": id} | {"change": key}]}.
func (h *Handlers) httpPutDependencies(c *gin.Context) {
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxSettingsBodyBytes))
	if err != nil {
		h.respondError(c, bodyErr("", "request body is unreadable or too large"))
		return
	}
	var body struct {
		Revision *int     `json:"dependency_revision"`
		Refs     []DepRef `json:"refs"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		h.respondError(c, bodyErr("", "body must be a JSON object"))
		return
	}
	expected := 0
	if body.Revision != nil {
		expected = *body.Revision
	}
	set, err := h.service.SaveDependencies(c.Request.Context(), c.Param("id"), c.Param("cid"), c.Param("tid"), body.Revision != nil, expected, body.Refs)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, set)
}
