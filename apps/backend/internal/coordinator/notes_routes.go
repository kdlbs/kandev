package coordinator

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// noteCodeKey is the JSON key of the reason code.
const noteCodeKey = "code"

// registerNoteRoutes adds the agent-tier note routes; they exist only while
// notes are effective.
func registerNoteRoutes(workspace *gin.RouterGroup, h *Handlers) {
	notes := workspace.Group("/coordinators/:cid/notes")
	notes.GET("", h.httpListNotes)
	notes.POST("", h.httpNoteMutation(noteOpCreate, http.StatusCreated))
	notes.GET("/:nid", h.httpGetNote)
	notes.GET("/:nid/versions/:ver", h.httpGetNoteVersion)
	notes.GET("/:nid/references", h.httpNoteReferences)
	notes.PUT("/:nid", h.httpNoteMutation(noteOpEdit, http.StatusOK))
	notes.POST("/:nid/revert", h.httpNoteMutation(noteOpRevert, http.StatusOK))
	notes.POST("/:nid/delete", h.httpNoteMutation(noteOpDelete, http.StatusOK))
}

// readNoteBody reads at most maxNoteBodyBytes; tooLarge reports an oversized
// body, which the service rejects after authorization.
func readNoteBody(c *gin.Context) (body []byte, tooLarge bool) {
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxNoteBodyBytes))
	var big *http.MaxBytesError
	if errors.As(err, &big) {
		return nil, true
	}
	return body, false
}

func (h *Handlers) httpNoteMutation(op string, okStatus int) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, tooLarge := readNoteBody(c)
		view, err := h.service.MutateNote(c.Request.Context(), op, c.Param("id"), c.Param("cid"), c.Param("nid"), body, tooLarge)
		if err != nil {
			h.respondNoteError(c, err)
			return
		}
		c.JSON(okStatus, view)
	}
}

func (h *Handlers) httpListNotes(c *gin.Context) {
	views, err := h.service.ListNotes(c.Request.Context(), c.Param("id"), c.Param("cid"), c.Query("include_deleted") == "true")
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"notes": views})
}

func (h *Handlers) httpGetNote(c *gin.Context) {
	detail, err := h.service.GetNote(c.Request.Context(), c.Param("id"), c.Param("cid"), c.Param("nid"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, detail)
}

func (h *Handlers) httpGetNoteVersion(c *gin.Context) {
	ver, convErr := strconv.Atoi(c.Param("ver"))
	if convErr != nil {
		h.respondError(c, ErrNotFound)
		return
	}
	v, err := h.service.NoteVersionDetail(c.Request.Context(), c.Param("id"), c.Param("cid"), c.Param("nid"), ver)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

func (h *Handlers) httpNoteReferences(c *gin.Context) {
	refs, err := h.service.NoteReferenceHistory(c.Request.Context(), c.Param("id"), c.Param("cid"), c.Param("nid"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"references": refs})
}

// respondNoteError renders a note rejection with its reason code; every other
// error uses the shared mapping.
func (h *Handlers) respondNoteError(c *gin.Context, err error) {
	var ne *NoteError
	if !errors.As(err, &ne) {
		h.respondError(c, err)
		return
	}
	body := gin.H{errorKey: ne.Code, noteCodeKey: ne.Code}
	if ne.Reason != "" {
		body["reason"] = ne.Reason
	}
	if ne.Field != "" {
		body["field"] = ne.Field
	}
	if ne.Index != nil {
		body["index"] = *ne.Index
	}
	c.JSON(ne.Status(), body)
}
