package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/clarification/protocol"
	"github.com/kandev/kandev/internal/task/models"
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

// scenarioAsyncQuestion exercises the opt-in nonblocking lifecycle: ask early,
// keep working, read the checkpoint, then wait explicitly for the answer.
func scenarioAsyncQuestion(e *emitter) {
	fixedDelay(100)
	e.text("Starting independent work while I ask an early question.")
	args := clarificationQuestionArgs()
	args["async"] = true
	args["request_id"] = "e2e-async-question-db"
	result, err := e.callMCPTool("kandev", "ask_user_question_kandev", args)
	if err != nil {
		e.text(fmt.Sprintf("Async question failed: %s", err))
		return
	}
	questionID := asyncQuestionID(result)
	e.text("ASYNC_RECEIPT " + result)

	fixedDelay(50)
	e.text("ASYNC_WORK_MARKER: continued independent work before the checkpoint read.")
	// Keep this foreground turn alive so the browser can answer while work is
	// still running. The result marker below proves the answer was read before
	// this turn completed, rather than delivered as a replacement prompt.
	waitForDelay(e.ctx, 30_000)
	listed, err := e.callMCPTool("kandev", "list_question_answers_kandev", map[string]any{"scope": "own"})
	if err != nil {
		e.text(fmt.Sprintf("Async checkpoint read failed: %s", err))
		return
	}
	e.text("ASYNC_CHECKPOINT " + listed)
	if label := asyncQuestionAnswerLabel(listed, questionID); label != "" {
		e.text("ASYNC_ANSWER_RECEIVED: " + label)
	}
	if questionID == "" {
		e.text("ASYNC_RECEIPT_SKIPPED: receipt carried no question id")
		return
	}
	e.text("ASYNC_CONTINUE_WITHOUT_WAIT")
}

func asyncQuestionID(result string) string {
	var receipt struct {
		QuestionID string `json:"question_id"`
	}
	if err := json.Unmarshal([]byte(result), &receipt); err != nil {
		return ""
	}
	return receipt.QuestionID
}

// scenarioAsyncQuestionWait exercises the explicit dependency wait: the agent
// declares that no independent work remains, so the captured turn parks until
// the question reaches a terminal outcome.
func scenarioAsyncQuestionWait(e *emitter) {
	fixedDelay(100)
	e.text("Starting independent work before an explicit question wait.")
	args := clarificationQuestionArgs()
	args["async"] = true
	args["request_id"] = "e2e-async-question-wait-db"
	result, err := e.callMCPTool("kandev", "ask_user_question_kandev", args)
	if err != nil {
		e.text(fmt.Sprintf("Async wait question failed: %s", err))
		return
	}
	questionID := asyncQuestionID(result)
	e.text("ASYNC_WAIT_WORK_MARKER " + result)
	if questionID == "" {
		e.text("ASYNC_WAIT_SKIPPED: receipt carried no question id")
		return
	}
	_, err = e.callMCPTool("kandev", "wait_for_question_answers_kandev", map[string]any{
		"question_ids": []string{questionID},
		"reason":       "e2e has no independent work left",
	})
	if err != nil {
		e.text(fmt.Sprintf("Async explicit wait failed: %s", err))
		return
	}
	listed, err := e.callMCPTool("kandev", "list_question_answers_kandev", map[string]any{
		"scope":        "own",
		"question_ids": []string{questionID},
	})
	if err != nil {
		e.text(fmt.Sprintf("Async wait answer read failed: %s", err))
		return
	}
	e.text("ASYNC_WAIT_REQUESTED")
	if label := asyncQuestionAnswerLabel(listed, questionID); label != "" {
		e.text("ASYNC_WAIT_ANSWER_RECEIVED: " + label)
	}
}

// scenarioAsyncParentQuestion creates a child-owned parent request, waits for
// its canonical outcome, and emits the selected label after the child resumes.
func scenarioAsyncParentQuestion(e *emitter) {
	fixedDelay(100)
	args := clarificationQuestionArgs()
	args["async"] = true
	args["request_id"] = "e2e-async-parent-question-db"
	result, err := e.callMCPTool("kandev", "ask_parent_question_kandev", args)
	if err != nil {
		e.text(fmt.Sprintf("Async parent question failed: %s", err))
		return
	}
	questionID := asyncQuestionID(result)
	if questionID == "" {
		e.text("ASYNC_PARENT_QUESTION_SKIPPED: receipt carried no question id")
		return
	}
	e.text("ASYNC_PARENT_WAIT_MARKER " + result)
	_, err = e.callMCPTool("kandev", "wait_for_question_answers_kandev", map[string]any{
		"question_ids": []string{questionID},
		"reason":       "e2e child work depends on the parent decision",
	})
	if err != nil {
		e.text(fmt.Sprintf("Async parent wait failed: %s", err))
		return
	}
	listed, err := e.callMCPTool("kandev", "list_question_answers_kandev", map[string]any{
		"scope":        "own",
		"question_ids": []string{questionID},
	})
	if err != nil {
		e.text(fmt.Sprintf("Async parent outcome read failed: %s", err))
		return
	}
	if label := asyncQuestionAnswerLabel(listed, questionID); label != "" {
		e.text("ASYNC_PARENT_ANSWER_RECEIVED: " + label)
	}
}

// scenarioAsyncParentForwardWatch is run in the direct parent task. It polls
// only its direct children, forwards the first pending request, and reports
// the canonical outcome after the operator answers from the parent's chat.
func scenarioAsyncParentForwardWatch(e *emitter) {
	questionIDs := []string(nil)
	for range 120 {
		listed, err := e.callMCPTool("kandev", "list_question_answers_kandev", map[string]any{
			"scope": "children", "status": "pending", "limit": 200,
		})
		if err != nil {
			e.text(fmt.Sprintf("Async child question read failed: %s", err))
			return
		}
		questionIDs = pendingAsyncQuestionIDs(listed)
		if len(questionIDs) >= 2 {
			break
		}
		waitForDelay(e.ctx, 250)
	}
	if len(questionIDs) < 2 {
		e.text("ASYNC_PARENT_FORWARD_SKIPPED: expected two pending child questions")
		return
	}
	questionIDs = questionIDs[:2]
	for _, questionID := range questionIDs {
		if _, err := e.callMCPTool("kandev", "forward_question_kandev", map[string]any{
			"question_id": questionID,
			"reason":      "e2e parent chat forwarding",
		}); err != nil {
			e.text(fmt.Sprintf("Async child question forward failed: %s", err))
			return
		}
		e.text("ASYNC_PARENT_FORWARDED: " + questionID)
	}
	resolved := make(map[string]bool, len(questionIDs))
	for range 120 {
		allResolved := true
		for _, questionID := range questionIDs {
			listed, err := e.callMCPTool("kandev", "list_question_answers_kandev", map[string]any{
				"scope": "children", "question_ids": []string{questionID},
			})
			if err != nil {
				e.text(fmt.Sprintf("Async child outcome read failed: %s", err))
				return
			}
			if label := asyncQuestionAnswerLabel(listed, questionID); label != "" {
				if !resolved[questionID] {
					e.text("ASYNC_PARENT_FORWARD_ANSWER_RECEIVED: " + label)
					resolved[questionID] = true
				}
			} else {
				allResolved = false
			}
		}
		if allResolved {
			return
		}
		waitForDelay(e.ctx, 250)
	}
	e.text("ASYNC_PARENT_FORWARD_ANSWER_SKIPPED: unresolved child question")
}

// asyncQuestionDeliveryResult turns a correlated outcome prompt into a stable
// E2E marker. Explicit waits cancel their captured turn, so the answer arrives
// in a new prompt rather than resuming the old scenario stack.
func asyncQuestionDeliveryResult(prompt string) string {
	marker, prefix := asyncQuestionDeliveryMarker(prompt)
	if marker == "" {
		return ""
	}
	messageStart := strings.LastIndex(prompt, marker)
	if messageStart < 0 {
		return ""
	}
	questions, outcome, ok := parseAsyncQuestionOutcome(prompt[messageStart:])
	if !ok {
		return ""
	}
	label := asyncQuestionOutcomeLabel(questions, outcome)
	if label == "" {
		return ""
	}
	return prefix + label
}

func asyncQuestionDeliveryMarker(prompt string) (string, string) {
	switch {
	case strings.Contains(prompt, "ASYNC CHILD QUESTION RESOLVED"):
		return "ASYNC CHILD QUESTION RESOLVED", "ASYNC_PARENT_ANSWER_RECEIVED: "
	case strings.Contains(prompt, "ASYNC QUESTION ANSWERED"):
		return "ASYNC QUESTION ANSWERED", "ASYNC_ANSWER_RECEIVED: "
	default:
		return "", ""
	}
}

func parseAsyncQuestionOutcome(message string) ([]protocol.Question, models.AsyncQuestionOutcome, bool) {
	const questionsHeader = "Questions and options:\n"
	const outcomeHeader = "Outcome:\n"
	questionStart := strings.Index(message, questionsHeader)
	outcomeStart := strings.Index(message, outcomeHeader)
	if questionStart < 0 || outcomeStart < questionStart {
		return nil, models.AsyncQuestionOutcome{}, false
	}
	var questions []protocol.Question
	if err := json.NewDecoder(strings.NewReader(message[questionStart+len(questionsHeader) : outcomeStart])).Decode(&questions); err != nil {
		return nil, models.AsyncQuestionOutcome{}, false
	}
	var outcome models.AsyncQuestionOutcome
	if err := json.NewDecoder(strings.NewReader(message[outcomeStart+len(outcomeHeader):])).Decode(&outcome); err != nil {
		return nil, models.AsyncQuestionOutcome{}, false
	}
	return questions, outcome, true
}

func asyncQuestionOutcomeLabel(questions []protocol.Question, outcome models.AsyncQuestionOutcome) string {
	for _, answer := range outcome.Answers {
		if answer.CustomText != "" {
			return answer.CustomText
		}
		if label := asyncQuestionAnswerOptionLabel(questions, answer); label != "" {
			return label
		}
	}
	return outcome.Text
}

func asyncQuestionAnswerOptionLabel(questions []protocol.Question, answer protocol.Answer) string {
	for _, question := range questions {
		if question.ID == answer.QuestionID {
			return asyncQuestionSelectedOptionLabel(question, answer.SelectedOptions)
		}
	}
	return ""
}

func asyncQuestionSelectedOptionLabel(question protocol.Question, selectedOptions []string) string {
	for _, selected := range selectedOptions {
		for _, option := range question.Options {
			if option.ID == selected {
				return option.Label
			}
		}
	}
	return ""
}

func scenarioAsyncQuestionWithdraw(e *emitter) {
	args := clarificationQuestionArgs()
	args["async"] = true
	args["request_id"] = "e2e-async-question-withdraw-db"
	receipt, err := e.callMCPTool("kandev", "ask_user_question_kandev", args)
	if err != nil {
		e.text(fmt.Sprintf("Async withdrawal question failed: %s", err))
		return
	}
	questionID := asyncQuestionID(receipt)
	if questionID == "" {
		e.text("ASYNC_WITHDRAW_SKIPPED: receipt carried no question id")
		return
	}
	if _, err := e.callMCPTool("kandev", "withdraw_question_kandev", map[string]any{
		"question_id": questionID,
		"reason":      "Found the database choice in the project docs.",
	}); err != nil {
		e.text(fmt.Sprintf("Async withdrawal failed: %s", err))
		return
	}
	e.text("ASYNC_WITHDRAWN: " + questionID)
}

func pendingAsyncQuestionIDs(result string) []string {
	var page struct {
		Questions []struct {
			QuestionID string `json:"question_id"`
			Status     string `json:"status"`
		} `json:"questions"`
	}
	if err := json.Unmarshal([]byte(result), &page); err != nil {
		return nil
	}
	ids := make([]string, 0, len(page.Questions))
	for _, question := range page.Questions {
		if question.Status == "pending" && question.QuestionID != "" {
			ids = append(ids, question.QuestionID)
		}
	}
	return ids
}

func asyncQuestionAnswerLabel(result, questionID string) string {
	var page struct {
		Questions []asyncQuestionAnswerRecord `json:"questions"`
	}
	if err := json.Unmarshal([]byte(result), &page); err != nil {
		return ""
	}
	for _, question := range page.Questions {
		if question.QuestionID != questionID {
			continue
		}
		return asyncQuestionOutcomeRecordLabel(question)
	}
	return ""
}

type asyncQuestionAnswerRecord struct {
	QuestionID string                       `json:"question_id"`
	Questions  []protocol.Question          `json:"questions"`
	Outcome    *models.AsyncQuestionOutcome `json:"outcome"`
}

func asyncQuestionOutcomeRecordLabel(question asyncQuestionAnswerRecord) string {
	if question.Outcome == nil {
		return ""
	}
	return asyncQuestionOutcomeLabel(question.Questions, *question.Outcome)
}
