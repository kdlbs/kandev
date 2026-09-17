package runtime

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

var ErrAssistantDisabled = errors.New("personal_assistant_disabled")

func (h *Handler) requireAssistant(c *gin.Context) {
	if !h.Service.AssistantEnabled {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{errorResponseKey: ErrAssistantDisabled.Error()})
		return
	}
	c.Next()
}

// CheckConversationExecution preserves the retained-owner boundary for native
// launches as well as runtime calls. Unowned coordinator conversations remain usable.
func (s *Service) CheckConversationExecution(ctx context.Context, taskID string) error {
	_, _, err := s.bindingSnapshot(ctx, taskID)
	return err
}
