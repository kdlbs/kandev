// Package previewfeedback validates task-owned rendered-page feedback at every
// persistence and delivery boundary.
package previewfeedback

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	MaxItems                        = 100
	MaxCommentBytes                 = 64 * 1024
	MaxCaptureFieldBytes            = 256 * 1024
	MaxCollectionPayloadBytes       = 1024 * 1024
	MaxScreenshots                  = 10
	MaxScreenshotBytes        int64 = 10 * 1024 * 1024
	MaxScreenshotPixels       int64 = 16_000_000
	MaxPromptBytes                  = 1024 * 1024
	MaxSourceLabelBytes             = 4 * 1024
	MaxPageIdentityBytes            = 16 * 1024
)

var (
	ErrCommentRequired     = errors.New("preview feedback comment is required")
	ErrCommentTooLarge     = errors.New("preview feedback comment is too large")
	ErrCaptureTooLarge     = errors.New("preview feedback capture is too large")
	ErrCaptureInvalid      = errors.New("preview feedback capture is invalid")
	ErrCollectionTooLarge  = errors.New("preview feedback collection is too large")
	ErrTooManyItems        = errors.New("too many preview feedback items")
	ErrTooManyScreenshots  = errors.New("too many preview feedback screenshots")
	ErrSourceIdentityLarge = errors.New("preview feedback source identity is too large")
)

func ValidateComment(comment string) error {
	if strings.TrimSpace(comment) == "" {
		return ErrCommentRequired
	}
	if len(comment) > MaxCommentBytes {
		return ErrCommentTooLarge
	}
	return nil
}

// ValidateItem enforces kind-specific capture fields and bounded source data.
func ValidateItem(item *models.TaskPreviewFeedback) error {
	if item == nil {
		return ErrCaptureInvalid
	}
	if err := ValidateComment(item.Comment); err != nil {
		return err
	}
	if err := validateSource(item); err != nil {
		return err
	}
	if err := validateOptionalJSON(item.CaptureRect); err != nil {
		return err
	}
	switch item.Kind {
	case models.TaskPreviewFeedbackText:
		return validateTextItem(item)
	case models.TaskPreviewFeedbackElement:
		return validateElementItem(item)
	case models.TaskPreviewFeedbackScreenshot:
		return validateScreenshotItem(item)
	default:
		return ErrCaptureInvalid
	}
}

func validateSource(item *models.TaskPreviewFeedback) error {
	if strings.TrimSpace(item.SourceLabel) == "" || strings.TrimSpace(item.PageRoute) == "" {
		return ErrCaptureInvalid
	}
	if len(item.SourceLabel) > MaxSourceLabelBytes || len(item.SourcePath) > MaxPageIdentityBytes ||
		len(item.PageRoute) > MaxPageIdentityBytes || len(item.PageTitle) > MaxPageIdentityBytes {
		return ErrSourceIdentityLarge
	}
	switch item.SourceKind {
	case models.TaskPreviewFeedbackBrowser:
		if item.SourcePath != "" {
			return ErrCaptureInvalid
		}
	case models.TaskPreviewFeedbackHTMLFile:
		if strings.TrimSpace(item.SourcePath) == "" {
			return ErrCaptureInvalid
		}
	default:
		return ErrCaptureInvalid
	}
	return nil
}

func validateTextItem(item *models.TaskPreviewFeedback) error {
	if strings.TrimSpace(item.SelectedText) == "" || len(item.TextAnchor) == 0 ||
		len(item.ElementSnapshot) != 0 || item.ScreenshotAttachmentID != "" {
		return ErrCaptureInvalid
	}
	if len(item.SelectedText) > MaxCaptureFieldBytes {
		return ErrCaptureTooLarge
	}
	return validateRequiredJSON(item.TextAnchor)
}

func validateElementItem(item *models.TaskPreviewFeedback) error {
	if item.SelectedText != "" || len(item.TextAnchor) != 0 || len(item.ElementSnapshot) == 0 ||
		len(item.CaptureRect) == 0 || item.ScreenshotAttachmentID != "" {
		return ErrCaptureInvalid
	}
	return validateRequiredJSON(item.ElementSnapshot)
}

func validateScreenshotItem(item *models.TaskPreviewFeedback) error {
	if item.SelectedText != "" || len(item.TextAnchor) != 0 || len(item.ElementSnapshot) != 0 ||
		len(item.CaptureRect) == 0 || item.ScreenshotAttachmentID == "" {
		return ErrCaptureInvalid
	}
	return nil
}

// ValidateCollection applies count and aggregate non-image payload limits.
func ValidateCollection(items []*models.TaskPreviewFeedback) error {
	if len(items) > MaxItems {
		return ErrTooManyItems
	}
	total := 0
	screenshots := 0
	for _, item := range items {
		if err := ValidateItem(item); err != nil {
			return err
		}
		if item.Kind == models.TaskPreviewFeedbackScreenshot {
			screenshots++
		}
		total += itemPayloadBytes(item)
	}
	if screenshots > MaxScreenshots {
		return ErrTooManyScreenshots
	}
	if total > MaxCollectionPayloadBytes {
		return ErrCollectionTooLarge
	}
	return nil
}

func validateRequiredJSON(value json.RawMessage) error {
	if len(value) == 0 {
		return ErrCaptureInvalid
	}
	return validateOptionalJSON(value)
}

func validateOptionalJSON(value json.RawMessage) error {
	if len(value) == 0 {
		return nil
	}
	if len(value) > MaxCaptureFieldBytes {
		return ErrCaptureTooLarge
	}
	if !json.Valid(value) {
		return fmt.Errorf("%w: malformed JSON", ErrCaptureInvalid)
	}
	return nil
}

func itemPayloadBytes(item *models.TaskPreviewFeedback) int {
	return len(item.Comment) + len(item.SourceSessionID) + len(item.SourceLabel) +
		len(item.SourcePath) + len(item.PageRoute) + len(item.PageTitle) +
		len(item.SelectedText) + len(item.TextAnchor) + len(item.ElementSnapshot) +
		len(item.CaptureRect)
}
