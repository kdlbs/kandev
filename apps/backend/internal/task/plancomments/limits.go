package plancomments

import (
	"errors"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	// MaxBodyBytes keeps one feedback item small enough for responsive editing
	// and bounded full-snapshot broadcasts.
	MaxBodyBytes = 64 << 10
	// MaxSelectedTextBytes permits substantial plan excerpts without allowing
	// one anchor to dominate every snapshot and rendered prompt.
	MaxSelectedTextBytes = 256 << 10
	// MaxPendingComments bounds full-snapshot mutation traffic.
	MaxPendingComments = 100
	// MaxAggregateBytes bounds bodies plus selected excerpts held by one plan.
	MaxAggregateBytes = 1 << 20
	// MaxRenderedPromptBytes matches the direct-message request ceiling after
	// server-owned comment formatting has expanded the submitted content.
	MaxRenderedPromptBytes = 1 << 20
)

var (
	ErrBodyTooLarge       = errors.New("plan comment body is too large")
	ErrSelectionTooLarge  = errors.New("plan comment selected text is too large")
	ErrCollectionTooLarge = errors.New("task plan comment collection is too large")
	ErrRenderedTooLarge   = errors.New("rendered plan comment prompt is too large")
)

// ValidateFields enforces limits knowable without reading the task snapshot.
func ValidateFields(body, selectedText string) error {
	if len(body) > MaxBodyBytes {
		return ErrBodyTooLarge
	}
	if len(selectedText) > MaxSelectedTextBytes {
		return ErrSelectionTooLarge
	}
	return nil
}

// ValidateCollection enforces the task-wide pending count and byte budget.
func ValidateCollection(comments []*models.TaskPlanComment) error {
	if len(comments) > MaxPendingComments {
		return ErrCollectionTooLarge
	}
	total := 0
	for _, comment := range comments {
		if comment == nil {
			continue
		}
		total += len(comment.Body) + len(comment.SelectedText)
		if total > MaxAggregateBytes {
			return ErrCollectionTooLarge
		}
	}
	return nil
}

// ValidateRenderedPrompt applies the final bound after server formatting.
func ValidateRenderedPrompt(content string) error {
	if len(content) > MaxRenderedPromptBytes {
		return ErrRenderedTooLarge
	}
	return nil
}
