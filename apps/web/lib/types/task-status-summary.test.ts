import { describe, expect, it } from "vitest";
import type { TaskStatusSummary } from "./task-status-summary";

describe("TaskStatusSummary", () => {
  it("models the bounded replacement payload without transcript fields", () => {
    const summary: TaskStatusSummary = {
      revision: 4,
      updated_at: "2026-08-01T18:56:13.512Z",
      primary_session: { id: "session-1", state: "WAITING_FOR_INPUT" },
      pending_action: "clarification",
      active_error: {
        session_id: "session-1",
        stamp: "error-4",
        occurred_at: "2026-08-01T18:56:13.512Z",
        preview: "The agent needs attention",
      },
      git: { additions: 3, deletions: 1, changed_files: 2 },
      pull_request: { count: 1, open_count: 1, attention: true, number: 42 },
    };

    expect(summary).not.toHaveProperty("messages");
    expect(summary).not.toHaveProperty("files");
  });
});
