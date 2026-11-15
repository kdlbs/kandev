package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// scenarioClarification: happy path — ask a question via MCP and wait for the answer.
func scenarioClarification(e *emitter) {
	fixedDelay(100)
	e.text("Let me ask you a question about the project setup.")

	result, err := e.callMCPTool("kandev", "ask_user_question_kandev", clarificationQuestionArgs())
	if err != nil {
		e.text(fmt.Sprintf("Question failed: %s", err))
		return
	}

	fixedDelay(50)
	e.text(fmt.Sprintf("You answered: %s", result))
}

// scenarioClarificationAttachments proves answer files reached the agent as
// readable bytes, not merely as echoed filenames. It is the browser-level
// counterpart to the backend materialization tests.
func scenarioClarificationAttachments(e *emitter) {
	fixedDelay(100)
	e.text("Let me ask a question and inspect any attached evidence.")

	result, err := e.callMCPTool("kandev", "ask_user_question_kandev", clarificationQuestionArgs())
	if err != nil {
		e.text(fmt.Sprintf("Question failed: %s", err))
		return
	}
	e.text("Attachment proof: " + clarificationAttachmentProof(result))
}

func scenarioClarificationAttachmentsChoiceOnly(e *emitter) {
	fixedDelay(100)
	e.text("Let me ask a choice-only question and inspect any attached evidence.")
	result, err := e.callMCPTool("kandev", "ask_user_question_kandev", clarificationNoOtherQuestionArgs())
	if err != nil {
		e.text(fmt.Sprintf("Question failed: %s", err))
		return
	}
	e.text("Attachment proof: " + clarificationAttachmentProof(result))
}

type clarificationAttachmentProofItem struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func clarificationAttachmentProof(result string) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		return "invalid-result"
	}
	var attachments []clarificationAttachmentProofItem
	collect := func(value any) {
		entry, ok := value.(map[string]any)
		if !ok {
			return
		}
		raw, ok := entry["attachments"].([]any)
		if !ok {
			return
		}
		for _, item := range raw {
			encoded, _ := json.Marshal(item)
			var attachment clarificationAttachmentProofItem
			if json.Unmarshal(encoded, &attachment) == nil && attachment.Path != "" {
				attachments = append(attachments, attachment)
			}
		}
	}
	for _, value := range payload {
		collect(value)
	}
	if len(attachments) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		data, err := os.ReadFile(attachment.Path)
		if err != nil {
			parts = append(parts, fmt.Sprintf("%s unreadable", attachment.Name))
			continue
		}
		sum := sha256.Sum256(data)
		parts = append(parts, fmt.Sprintf("%s %d %s", attachment.Name, len(data), hex.EncodeToString(sum[:])))
	}
	return strings.Join(parts, "; ")
}

func scenarioClarificationNoOther(e *emitter) {
	fixedDelay(100)
	e.text("Please choose one of the offered modes.")
	result, err := e.callMCPTool("kandev", "ask_user_question_kandev", clarificationNoOtherQuestionArgs())
	if err != nil {
		e.text(fmt.Sprintf("Question failed: %s", err))
		return
	}
	fixedDelay(50)
	e.text(fmt.Sprintf("You answered: %s", result))
}

// scenarioClarificationMarkdown exercises the restricted Markdown renderer
// through the same blocking MCP round trip used by real clarification calls.
func scenarioClarificationMarkdown(e *emitter) {
	fixedDelay(100)
	e.text("Let me ask you a formatted question about project storage.")

	result, err := e.callMCPTool("kandev", "ask_user_question_kandev", clarificationMarkdownQuestionArgs())
	if err != nil {
		e.text(fmt.Sprintf("Question failed: %s", err))
		return
	}

	fixedDelay(50)
	e.text(fmt.Sprintf("You answered: %s", result))
}

// scenarioClarificationMulti: stress the multi-question path — 3 questions in
// one bundle, all required, single MCP call.
func scenarioClarificationMulti(e *emitter) {
	fixedDelay(100)
	e.text("Let me ask you a few questions about the project setup.")

	result, err := e.callMCPTool("kandev", "ask_user_question_kandev", clarificationMultiQuestionArgs())
	if err != nil {
		e.text(fmt.Sprintf("Questions failed: %s", err))
		return
	}

	fixedDelay(50)
	e.text(fmt.Sprintf("You answered: %s", result))
}

// scenarioClarificationTimeout: ask a question with a short timeout, then continue.
func scenarioClarificationTimeout(e *emitter) {
	fixedDelay(100)
	e.text("Let me ask you a question about the project setup.")

	ctx, cancel := contextWithTimeout(5)
	defer cancel()

	result, err := e.callMCPToolCtx(ctx, "kandev", "ask_user_question_kandev", clarificationQuestionArgs())
	if err != nil {
		if ctx.Err() != nil || strings.Contains(strings.ToLower(err.Error()), "timeout") {
			// The backend cancels the in-flight ACP prompt when the MCP client
			// times out. Keep the mock's explicit continuation update on the
			// live transport so this scenario exercises the late-event
			// admission path rather than dropping it with the canceled prompt
			// context.
			continuation := *e
			continuation.ctx = context.WithoutCancel(e.ctx)
			continuation.text("Question timed out, continuing without answer.")
		} else {
			fixedDelay(50)
			e.text(fmt.Sprintf("Question failed: %s", err))
		}
		return
	}

	fixedDelay(50)
	e.text(fmt.Sprintf("You answered: %s", result))
}

func scenarioClarificationContextParagraphs(e *emitter) {
	fixedDelay(100)
	e.text("Let me ask a storage question with shared context paragraphs.")
	result, err := e.callMCPTool("kandev", "ask_user_question_kandev", clarificationContextParagraphsQuestionArgs())
	if err != nil {
		e.text(fmt.Sprintf("Question failed: %s", err))
		return
	}
	fixedDelay(50)
	e.text(fmt.Sprintf("You answered: %s", result))
}
