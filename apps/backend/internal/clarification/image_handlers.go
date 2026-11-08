package clarification

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/clarification/protocol"
	"github.com/kandev/kandev/internal/common/imagepreview"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// QuestionImageReader resolves files only through the authorized source session.
type QuestionImageReader interface {
	ReadQuestionImage(context.Context, string, protocol.QuestionImage) (*imagepreview.Content, error)
}

func (h *Handlers) httpQuestionImage(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	sessionID, taskID, err := h.resolver.AuthorizeBundleAccess(ctx, c.Param("id"))
	if err != nil {
		writeQuestionImageError(c, imagepreview.NotFound)
		return
	}
	ref, err := h.savedQuestionImage(ctx, c.Param("id"), c.Query("question_id"), c.Param("imageID"), sessionID, taskID)
	if err != nil {
		writeQuestionImageError(c, imagepreview.NotFound)
		return
	}
	if h.imageReader == nil {
		writeQuestionImageError(c, imagepreview.Unavailable)
		return
	}
	content, err := h.imageReader.ReadQuestionImage(ctx, sessionID, ref)
	if err != nil {
		code := imagepreview.Unavailable
		_ = errors.As(err, &code)
		writeQuestionImageError(c, code)
		return
	}
	if content == nil || len(content.Bytes) > imagepreview.MaxBytes || (content.MIME != "image/png" && content.MIME != "image/jpeg") {
		writeQuestionImageError(c, imagepreview.InvalidImage)
		return
	}
	c.Header("Content-Disposition", "inline")
	c.Data(http.StatusOK, content.MIME, content.Bytes)
}

func writeQuestionImageError(c *gin.Context, code imagepreview.Error) {
	c.JSON(code.Status(), gin.H{"code": code})
}

func (h *Handlers) savedQuestionImage(ctx context.Context, pendingID, questionID, imageID, sessionID, taskID string) (protocol.QuestionImage, error) {
	session, err := h.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID != taskID {
		return protocol.QuestionImage{}, imagepreview.NotFound
	}
	messages, err := h.repo.FindMessagesByPendingID(ctx, pendingID)
	if err != nil || len(messages) == 0 || isParentQuestionBundle(messages) {
		return protocol.QuestionImage{}, imagepreview.NotFound
	}
	if !questionImageMessagesMatch(messages, sessionID, taskID) {
		return protocol.QuestionImage{}, imagepreview.NotFound
	}
	questions := bundleQuestions(messages)
	if err = protocol.ValidateQuestionImages(questions); err != nil {
		return protocol.QuestionImage{}, imagepreview.NotFound
	}
	for _, question := range questions {
		if question.ID != questionID {
			continue
		}
		for _, ref := range question.ImageReferences() {
			if ref.ID == imageID {
				return ref, nil
			}
		}
	}
	return protocol.QuestionImage{}, imagepreview.NotFound
}

func questionImageMessagesMatch(messages []*taskmodels.Message, sessionID, taskID string) bool {
	for _, message := range messages {
		if message.TaskSessionID != sessionID || message.TaskID != taskID {
			return false
		}
		if id := stringFromMetadata(message.Metadata, metaSessionIDKey); id != "" && id != sessionID {
			return false
		}
		if id := stringFromMetadata(message.Metadata, metaTaskIDKey); id != "" && id != taskID {
			return false
		}
	}
	return true
}
