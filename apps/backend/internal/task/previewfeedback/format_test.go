package previewfeedback

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestAppendMarkdownIncludesRuntimeTextLocationAndEscapesFences(t *testing.T) {
	item := &models.TaskPreviewFeedback{
		Kind: models.TaskPreviewFeedbackText, Comment: "Generated at runtime",
		SourceKind: models.TaskPreviewFeedbackBrowser, SourceLabel: "Local app",
		PageRoute: "/products?tab=all", PageTitle: "Products",
		SelectedText: "Choose `this` plan",
		TextAnchor: json.RawMessage(`{
			"start":{"selector":"#offer","node_path":[0,1],"offset":2},
			"end":{"selector":"#offer","node_path":[0,1],"offset":20},
			"rects":[{"x":20,"y":30,"width":160,"height":20}],
			"union_rect":{"x":20,"y":30,"width":160,"height":20},
			"scroll_x":0,"scroll_y":480,"viewport_width":1280,"viewport_height":720,
			"device_pixel_ratio":2,
			"containing_element":{"tag":"button","classes":["offer"],"outer_html":"<button>` + "```generated```" + `</button>"}
		}`),
	}

	got, err := AppendMarkdown("Please fix this.", []*models.TaskPreviewFeedback{item})
	if err != nil {
		t.Fatalf("AppendMarkdown: %v", err)
	}
	for _, want := range []string{
		"Please fix this.", "### Web Preview Feedback", "Text feedback",
		`Source: "Local app"`, `Page route: "/products?tab=all"`,
		"Choose `this` plan", "containing_element", "node_path", "union_rect",
		"scroll_y", "viewport_width", "device_pixel_ratio", "````json",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted prompt missing %q:\n%s", want, got)
		}
	}
}

func TestAppendMarkdownFormatsElementAndScreenshot(t *testing.T) {
	items := []*models.TaskPreviewFeedback{
		{
			Kind: models.TaskPreviewFeedbackElement, Comment: "Align with price",
			SourceKind: models.TaskPreviewFeedbackHTMLFile, SourceLabel: "checkout.html",
			SourcePath: "mockups/checkout.html", PageRoute: "/checkout", PageTitle: "Checkout",
			ElementSnapshot: json.RawMessage(`{"tag":"button","selector":"button.checkout","outer_html":"<button class=\"checkout\">Pay</button>"}`),
			CaptureRect:     json.RawMessage(`{"x":12,"y":24,"width":80,"height":32}`),
		},
		{
			Kind: models.TaskPreviewFeedbackScreenshot, Comment: "The form clips here",
			SourceKind: models.TaskPreviewFeedbackBrowser, SourceLabel: "Local app",
			PageRoute: "/account", PageTitle: "Account",
			CaptureRect:            json.RawMessage(`{"x":10,"y":20,"width":820,"height":360}`),
			ScreenshotAttachmentID: "shot-1",
			ScreenshotAttachment: &models.TaskMessageAttachment{
				ID: "shot-1", Name: "account-form.png", MimeType: "image/png", SizeBytes: 1234,
			},
		},
	}

	got, err := AppendMarkdown("", items)
	if err != nil {
		t.Fatalf("AppendMarkdown: %v", err)
	}
	for _, want := range []string{
		"Element feedback", `<button class=\"checkout\">Pay</button>`,
		"Rendered position", "Screenshot feedback", `Image: "account-form.png"`,
		"820", "360", "The form clips here",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted prompt missing %q:\n%s", want, got)
		}
	}
}

func TestAppendMarkdownRejectsOversizedPrompt(t *testing.T) {
	_, err := AppendMarkdown(strings.Repeat("x", MaxPromptBytes), []*models.TaskPreviewFeedback{{
		Kind: models.TaskPreviewFeedbackElement, Comment: "comment",
		SourceKind: models.TaskPreviewFeedbackBrowser, SourceLabel: "app",
		PageRoute: "/", ElementSnapshot: json.RawMessage(`{"outer_html":"<main></main>"}`),
		CaptureRect: json.RawMessage(`{"x":0,"y":0,"width":1,"height":1}`),
	}})
	if err == nil {
		t.Fatal("AppendMarkdown accepted an oversized prompt")
	}
}
