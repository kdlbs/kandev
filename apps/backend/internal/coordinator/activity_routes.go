package coordinator

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"
)

// singleQuery returns the value of a query key that appears at most once.
// present is false when the key is absent. A repeated key is a FieldError.
func singleQuery(q url.Values, key string) (value string, present bool, err error) {
	values, ok := q[key]
	if !ok {
		return "", false, nil
	}
	if len(values) != 1 {
		return "", true, &FieldError{Field: key, Message: key + " must appear once"}
	}
	return values[0], true, nil
}

// intQuery parses an integer query key; absent yields def, an empty or
// non-integer value is a FieldError naming the key.
func intQuery(q url.Values, key string, def int) (int, error) {
	value, present, err := singleQuery(q, key)
	if err != nil {
		return 0, err
	}
	if !present {
		return def, nil
	}
	n, convErr := strconv.Atoi(value)
	if convErr != nil {
		return 0, &FieldError{Field: key, Message: key + " must be an integer"}
	}
	return n, nil
}

func parseListActivityQuery(q url.Values) (ListActivityParams, error) {
	var p ListActivityParams
	class, _, err := singleQuery(q, "class")
	if err != nil {
		return p, err
	}
	p.Class = class
	before, present, err := singleQuery(q, "before")
	if err != nil {
		return p, err
	}
	if present && before == "" {
		return p, &FieldError{Field: "before", Message: "before must not be empty"}
	}
	p.Before = before
	limit, err := intQuery(q, "limit", DefaultActivityLimit)
	if err != nil {
		return p, err
	}
	p.Limit = limit
	if limit == 0 {
		return p, &FieldError{Field: "limit", Message: "limit must be between 1 and 50"}
	}
	return p, nil
}

// httpListActivity backs GET .../coordinators/:cid/activity.
func (h *Handlers) httpListActivity(c *gin.Context) {
	params, err := parseListActivityQuery(c.Request.URL.Query())
	if err != nil {
		h.respondError(c, err)
		return
	}
	page, err := h.service.ListActivity(c.Request.Context(), c.Param("id"), c.Param("cid"), params)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}

// httpActivitySummary backs GET .../coordinators/:cid/activity/summary.
func (h *Handlers) httpActivitySummary(c *gin.Context) {
	days, err := intQuery(c.Request.URL.Query(), "days", DefaultSummaryDays)
	if err != nil {
		h.respondError(c, err)
		return
	}
	summary, err := h.service.GetActivitySummary(c.Request.Context(), c.Param("id"), c.Param("cid"), days)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, summary)
}

// httpUndoActivity backs POST .../coordinators/:cid/activity/:rid/undo.
func (h *Handlers) httpUndoActivity(c *gin.Context) {
	item, err := h.service.UndoActivity(c.Request.Context(), c.Param("id"), c.Param("cid"), c.Param("rid"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}
