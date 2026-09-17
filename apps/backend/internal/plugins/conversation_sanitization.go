package plugins

import (
	"github.com/kandev/kandev/internal/sysprompt"
)

//nolint:goconst // These keys are an explicit privacy allowlist.
var conversationMessageMetadataKeys = map[string]struct{}{
	"action_details": {}, "action_type": {}, "action_visibility": {},
	"actions": {}, "agent_disconnected": {},
	"attempt": {}, "attachments": {}, "auth_methods": {}, "auto_start": {},
	"base_branch": {}, "context": {}, "context_files": {}, "decision_id": {},
	"effective_model": {}, "entity_references": {}, "error_output": {},
	"failure_code": {}, "failure_details": {}, "failure_kind": {},
	"fallback_model": {}, "has_hidden_prompts": {}, "has_resume_token": {},
	"has_review_comments": {}, "is_auth_error": {}, "kind": {}, "max_attempts": {},
	"message": {}, "missing_branch": {}, "model_id": {}, "new_branch": {},
	"original_branch": {}, "options": {}, "pending_id": {}, "plan_mode": {}, "progress": {},
	"provider_name": {}, "question": {}, "question_id": {}, "question_index": {},
	"question_total": {}, "recovery_actions": {}, "remediation": {},
	"remediation_url": {}, "requested_model": {}, "request_id": {},
	"requests_input": {}, "response": {}, "reset_at": {}, "retry_at": {},
	"retry_in_seconds": {}, "retrying": {}, "sender_session_id": {}, "sender_session_name": {},
	"sender_task_id": {}, "sender_task_title": {}, "stage": {}, "status": {},
	"script_type": {}, "agent_name": {}, "command": {}, "exit_code": {},
	"is_resuming": {}, "started_at": {}, "completed_at": {}, "error": {},
	"task_id": {}, "text": {}, "tool_call_id": {}, "variant": {}, "workflow_message": {},
	"workflow_step_color": {}, "workflow_step_id": {}, "workflow_step_name": {},
}

// SanitizeConversationMessageMetadata returns the presentation metadata that
// the plugin conversation stream may expose. Internal agent and tool metadata
// is excluded by the allowlist.
func SanitizeConversationMessageMetadata(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	target := make(map[string]any)
	for key := range conversationMessageMetadataKeys {
		value, exists := source[key]
		if !exists || value == nil {
			continue
		}
		target[key] = sanitizeConversationMetadataValue(value)
	}
	return target
}

func sanitizeConversationMetadataValue(value any) any {
	switch typed := value.(type) {
	case string:
		return sysprompt.StripSystemContent(typed)
	case map[string]any:
		target := make(map[string]any, len(typed))
		for key, nested := range typed {
			target[key] = sanitizeConversationMetadataValue(nested)
		}
		return target
	case []any:
		target := make([]any, len(typed))
		for index, nested := range typed {
			target[index] = sanitizeConversationMetadataValue(nested)
		}
		return target
	default:
		return value
	}
}
