import { it, expect } from "vitest";
import { resolveRecoveryHistoryState } from "./action-message-recovery-history";
import { sessionId, taskId, type Message } from "@/lib/types/http";

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.13
it("keeps resolved cleanup evidence in localized history without recovery controls", () => {
  const metadata = {
    recovery_actions: true,
    error_stamp: "cleanup-stamp",
    causes: [{ operation: "resume", code: "runtime_cleanup_failed" }],
  };
  const comment: Message = {
    id: "cleanup-message",
    session_id: sessionId("session"),
    task_id: taskId("task"),
    type: "status",
    author_type: "agent",
    content: "safe host cause",
    created_at: "2026-10-10T10:00:00Z",
    metadata,
  };
  const sessionMetadata = {
    last_agent_error: { message: comment.content, stamp: "cleanup-stamp" },
    recovery_resolutions: [
      { error_stamp: "cleanup-stamp", resolved_at: "2026-10-10T11:00:00Z", attempt_id: "resume-2" },
    ],
  };
  expect(resolveRecoveryHistoryState(comment, metadata, undefined, sessionMetadata)).toBe(true);
  expect(
    resolveRecoveryHistoryState(
      comment,
      { ...metadata, causes: [{ operation: "resume", code: "unknown" }] },
      undefined,
      sessionMetadata,
    ),
  ).toBeNull();
});
