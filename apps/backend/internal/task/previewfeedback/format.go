package previewfeedback

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

var ErrPromptTooLarge = errors.New("preview feedback prompt is too large")

const (
	MetadataRefs               = "preview_feedback_refs"
	MetadataRequestFingerprint = "preview_feedback_request_fingerprint"
)

// AppendMarkdown adds persisted rendered-page evidence as visible user content.
func AppendMarkdown(content string, items []*models.TaskPreviewFeedback) (string, error) {
	if len(items) == 0 {
		return "", errors.New("preview feedback items are required")
	}
	var out strings.Builder
	out.WriteString(content)
	if content != "" && !strings.HasSuffix(content, "\n\n") {
		if strings.HasSuffix(content, "\n") {
			out.WriteByte('\n')
		} else {
			out.WriteString("\n\n")
		}
	}
	out.WriteString("### Web Preview Feedback\n\n")
	for index, item := range items {
		if item == nil {
			return "", ErrCaptureInvalid
		}
		if err := appendItemMarkdown(&out, index, item); err != nil {
			return "", ErrCaptureInvalid
		}
	}
	if out.Len() > MaxPromptBytes {
		return "", ErrPromptTooLarge
	}
	return out.String(), nil
}

func appendItemMarkdown(out *strings.Builder, index int, item *models.TaskPreviewFeedback) error {
	fmt.Fprintf(out, "#### %d. %s feedback\n\n", index+1, formatKind(item.Kind))
	writeQuotedField(out, "Source", item.SourceLabel)
	writeQuotedField(out, "Source type", string(item.SourceKind))
	if item.SourcePath != "" {
		writeQuotedField(out, "Source path", item.SourcePath)
	}
	writeQuotedField(out, "Page route", item.PageRoute)
	if item.PageTitle != "" {
		writeQuotedField(out, "Page title", item.PageTitle)
	}
	out.WriteString("- Comment:\n")
	writeQuote(out, item.Comment)
	out.WriteByte('\n')

	if err := appendCaptureMarkdown(out, item); err != nil {
		return err
	}
	out.WriteByte('\n')
	return nil
}

func appendCaptureMarkdown(out *strings.Builder, item *models.TaskPreviewFeedback) error {
	switch item.Kind {
	case models.TaskPreviewFeedbackText:
		out.WriteString("- Selected text:\n")
		writeQuote(out, item.SelectedText)
		out.WriteString("\n- Rendered text anchor (containing element, DOM endpoints, rectangles, scroll, viewport, and pixel ratio):\n")
		writeJSONFence(out, item.TextAnchor)
	case models.TaskPreviewFeedbackElement:
		out.WriteString("- Rendered element snapshot:\n")
		writeJSONFence(out, item.ElementSnapshot)
		out.WriteString("- Rendered position:\n")
		writeJSONFence(out, item.CaptureRect)
	case models.TaskPreviewFeedbackScreenshot:
		writeQuotedField(out, "Image", screenshotName(item))
		out.WriteString("- Captured region and dimensions:\n")
		writeJSONFence(out, item.CaptureRect)
	default:
		return ErrCaptureInvalid
	}
	return nil
}

func screenshotName(item *models.TaskPreviewFeedback) string {
	if item.ScreenshotAttachment != nil && item.ScreenshotAttachment.Name != "" {
		return item.ScreenshotAttachment.Name
	}
	return item.ScreenshotAttachmentID
}

// MetadataRefsMatch verifies the persisted delivery identity for an exact retry.
func MetadataRefsMatch(metadata map[string]interface{}, refs []models.TaskPreviewFeedbackRef) bool {
	raw, exists := metadata[MetadataRefs]
	if !exists {
		return len(refs) == 0
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return false
	}
	var stored []models.TaskPreviewFeedbackRef
	if err := json.Unmarshal(data, &stored); err != nil || len(stored) != len(refs) {
		return false
	}
	for index := range refs {
		if stored[index] != refs[index] {
			return false
		}
	}
	return true
}

func formatKind(kind models.TaskPreviewFeedbackKind) string {
	switch kind {
	case models.TaskPreviewFeedbackText:
		return "Text"
	case models.TaskPreviewFeedbackElement:
		return "Element"
	case models.TaskPreviewFeedbackScreenshot:
		return "Screenshot"
	default:
		return "Unknown"
	}
}

func writeQuotedField(out *strings.Builder, label, value string) {
	fmt.Fprintf(out, "- %s: %s\n", label, strconv.Quote(value))
}

func writeQuote(out *strings.Builder, value string) {
	for _, line := range strings.Split(value, "\n") {
		out.WriteString("> ")
		out.WriteString(line)
		out.WriteByte('\n')
	}
}

func writeJSONFence(out *strings.Builder, raw json.RawMessage) {
	formatted := raw
	var pretty bytes.Buffer
	if json.Indent(&pretty, raw, "", "  ") == nil {
		formatted = pretty.Bytes()
	}
	fence := strings.Repeat("`", longestBacktickRun(string(formatted))+1)
	if len(fence) < 3 {
		fence = "```"
	}
	out.WriteString(fence)
	out.WriteString("json\n")
	out.Write(formatted)
	out.WriteByte('\n')
	out.WriteString(fence)
	out.WriteByte('\n')
}

func longestBacktickRun(value string) int {
	longest, current := 0, 0
	for _, char := range value {
		if char == '`' {
			current++
			if current > longest {
				longest = current
			}
		} else {
			current = 0
		}
	}
	return longest
}
