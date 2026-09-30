package coordinator

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

const fieldInclude = "include"

// registerStandingOrderRoutes adds the standing order routes, which exist only
// while phase 2 is on.
func registerStandingOrderRoutes(workspace *gin.RouterGroup, h *Handlers) {
	orders := workspace.Group("/coordinators/:cid/standing-orders")
	orders.GET("", h.httpListStandingOrders)
	orders.POST("", h.httpAddStandingOrder)
	orders.POST("/:oid/retire", h.httpRetireStandingOrder)
	orders.POST("/:oid/restore", h.httpRestoreStandingOrder)
}

// httpListStandingOrders backs GET .../standing-orders?include=retired.
func (h *Handlers) httpListStandingOrders(c *gin.Context) {
	includeRetired := false
	switch c.Query(fieldInclude) {
	case "":
	case "retired":
		includeRetired = true
	default:
		h.respondError(c, &FieldError{Field: fieldInclude, Message: "include must be retired"})
		return
	}
	orders, err := h.service.ListStandingOrders(c.Request.Context(), c.Param("id"), c.Param("cid"), includeRetired)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, &StandingOrderListResponse{Orders: orders})
}

// httpAddStandingOrder backs POST .../standing-orders.
func (h *Handlers) httpAddStandingOrder(c *gin.Context) {
	var body map[string]json.RawMessage
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, NewErrorResponse("invalid request body"))
		return
	}
	in, err := parseAddStandingOrder(body)
	if err != nil {
		h.respondError(c, err)
		return
	}
	order, err := h.service.AddStandingOrder(c.Request.Context(), c.Param("id"), c.Param("cid"), in)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, order)
}

// parseAddStandingOrder reads the add body: text is required and a string;
// source_proposal_id is absent when missing or null and must be a string
// otherwise.
func parseAddStandingOrder(body map[string]json.RawMessage) (AddStandingOrderInput, error) {
	text, present, err := rawStringField(body, fieldStandingOrderText)
	if err != nil {
		return AddStandingOrderInput{}, err
	}
	if !present {
		return AddStandingOrderInput{}, &FieldError{Field: fieldStandingOrderText, Message: "text is required"}
	}
	in := AddStandingOrderInput{Text: *text}
	if raw, ok := body[fieldSourceProposalID]; ok && string(raw) != jsonNullLiteral {
		id, _, err := rawStringField(body, fieldSourceProposalID)
		if err != nil {
			return AddStandingOrderInput{}, err
		}
		in.SourceProposalID = id
	}
	return in, nil
}

// httpRetireStandingOrder backs POST .../standing-orders/:oid/retire.
func (h *Handlers) httpRetireStandingOrder(c *gin.Context) {
	order, err := h.service.RetireStandingOrder(c.Request.Context(), c.Param("id"), c.Param("cid"), c.Param("oid"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, order)
}

// httpRestoreStandingOrder backs POST .../standing-orders/:oid/restore.
func (h *Handlers) httpRestoreStandingOrder(c *gin.Context) {
	order, err := h.service.RestoreStandingOrder(c.Request.Context(), c.Param("id"), c.Param("cid"), c.Param("oid"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, order)
}
