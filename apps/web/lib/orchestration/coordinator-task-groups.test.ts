import { describe, expect, it } from "vitest";
import type { Task } from "@/lib/types/http";
import { coordinatorTaskGroup } from "./coordinator-task-groups";

const task = (fields: Partial<Task> = {}) => ({ id: "t", state: "IN_PROGRESS", ...fields }) as Task;
const summary = { revision: 3, updated_at: "2026-09-17T00:00:00Z" };
describe("coordinatorTaskGroup", () => {
  it("pending input outranks running across sessions", () => {
    expect(
      coordinatorTaskGroup(
        task({ primary_session_state: "RUNNING", task_pending_action: "permission" }),
      ),
    ).toBe("input");
    expect(
      coordinatorTaskGroup(
        task({
          state: "COMPLETED",
          status_summary: { ...summary, pending_action: "clarification" },
        }),
      ),
    ).toBe("input");
  });
  it("a newer canonical summary clears an old DTO pending badge", () => {
    expect(
      coordinatorTaskGroup(task({ task_pending_action: "permission", status_summary: summary })),
    ).toBe("other");
  });
  it("an authoritative idle summary clears stale running fields", () => {
    expect(
      coordinatorTaskGroup(
        task({
          primary_session_state: "RUNNING",
          foreground_activity: "generating",
          status_summary: summary,
        }),
      ),
    ).toBe("other");
  });
  it("idle is not stalled and waiting alone is not a question", () => {
    expect(coordinatorTaskGroup(task())).toBe("other");
    expect(coordinatorTaskGroup(task({ primary_session_state: "WAITING_FOR_INPUT" }))).toBe(
      "other",
    );
  });
  it("merged PR is not task completion", () => {
    expect(
      coordinatorTaskGroup(
        task({ status_summary: { ...summary, pull_request: { state: "MERGED" } } }),
      ),
    ).toBe("other");
  });
  it("native errors outrank activity and acknowledged errors are quiet", () => {
    const row = task({
      status_summary: {
        ...summary,
        active_error: {
          session_id: "s",
          stamp: "err",
          preview: "Failed",
          occurred_at: summary.updated_at,
        },
      },
    });
    expect(coordinatorTaskGroup(row)).toBe("problems");
    expect(coordinatorTaskGroup(row, { s: "err" })).toBe("other");
    expect(
      coordinatorTaskGroup(task({ interrupted: true, foreground_activity: "background" })),
    ).toBe("problems");
  });
  it("projects live background, review, done and queued evidence", () => {
    expect(coordinatorTaskGroup(task({ foreground_activity: "background" }))).toBe("running");
    expect(
      coordinatorTaskGroup(
        task({ status_summary: { ...summary, pull_request: { attention: true } } }),
      ),
    ).toBe("review");
    expect(coordinatorTaskGroup(task({ state: "COMPLETED" }))).toBe("done");
    expect(
      coordinatorTaskGroup(task({ status_summary: { ...summary, queued_prompt_count: 2 } })),
    ).toBe("queued");
    expect(coordinatorTaskGroup(task({ state: "CREATED" }))).toBe("queued");
  });
});
