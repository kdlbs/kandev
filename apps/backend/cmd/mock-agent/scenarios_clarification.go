package main

import (
	"context"
	"fmt"
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
