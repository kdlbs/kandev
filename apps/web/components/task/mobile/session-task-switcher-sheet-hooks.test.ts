import { describe, expect, it } from "vitest";
import { toSheetItem } from "./session-task-switcher-sheet-hooks";

type SheetTask = Parameters<typeof toSheetItem>[0];
type SheetCtx = Parameters<typeof toSheetItem>[1];
const UPDATED_AT = "2026-07-22T00:00:00Z";
const ERROR_PREVIEW = "Agent failed";

function emptyCtx(): SheetCtx {
  return {
    repositoryPathsById: new Map(),
    workflowNameById: new Map(),
    stepTitleById: new Map(),
  };
}

function task(overrides: Partial<SheetTask> = {}): SheetTask {
  return {
    id: "t1",
    _workflowId: "wf1",
    title: "Task",
    state: "IN_PROGRESS",
    workflowStepId: "step-1",
    ...overrides,
  } as SheetTask;
}

describe("toSheetItem", () => {
  // The mobile task-switcher row must read the same task-level most-active-wins
  // aggregate the desktop sidebar and board card read, so a background-running
  // secondary session is caught on mobile too.
  it("carries the task-level foreground_activity aggregate onto the mobile sheet row", () => {
    const item = toSheetItem(task({ foregroundActivity: "background" }), emptyCtx());
    expect(item.foregroundActivity).toBe("background");
  });

  it("carries the generating aggregate through unchanged", () => {
    const item = toSheetItem(task({ foregroundActivity: "generating" }), emptyCtx());
    expect(item.foregroundActivity).toBe("generating");
  });

  it("passes an absent aggregate through as undefined (safe → not-background)", () => {
    const item = toSheetItem(task(), emptyCtx());
    expect(item.foregroundActivity).toBeUndefined();
  });

  it("reads pending permission from the task status summary", () => {
    const item = toSheetItem(
      task({
        statusSummary: {
          revision: 2,
          updated_at: UPDATED_AT,
          pending_action: "permission",
        },
      }),
      emptyCtx(),
    );
    expect(item.hasPendingPermission).toBe(true);
    expect(item.hasPendingClarification).toBe(false);
  });

  it("reads an active error from the task status summary", () => {
    const item = toSheetItem(
      task({
        statusSummary: {
          revision: 3,
          updated_at: UPDATED_AT,
          active_error: {
            session_id: "session-1",
            stamp: "error-3",
            occurred_at: UPDATED_AT,
            preview: ERROR_PREVIEW,
          },
        },
      }),
      emptyCtx(),
    );

    expect(item.agentErrorMessage).toBe(ERROR_PREVIEW);
  });

  it("does not resurrect legacy status when a summary explicitly clears it", () => {
    const item = toSheetItem(
      task({
        taskPendingAction: "permission",
        primarySessionState: "RUNNING",
        primarySessionId: "legacy-session",
        foregroundActivity: "background",
        updatedAt: "legacy-update",
        statusSummary: {
          revision: 4,
          updated_at: UPDATED_AT,
        },
      }),
      emptyCtx(),
    );

    expect(item.hasPendingPermission).toBe(false);
    expect(item.sessionState).toBeUndefined();
    expect(item.primarySessionId).toBeNull();
    expect(item.foregroundActivity).toBeUndefined();
    expect(item.updatedAt).toBe(UPDATED_AT);
  });

  it("hides only the acknowledged error stamp and shows a newer one", () => {
    const base = task({
      statusSummary: {
        revision: 5,
        updated_at: UPDATED_AT,
        active_error: {
          session_id: "session-1",
          stamp: "error-5",
          occurred_at: UPDATED_AT,
          preview: ERROR_PREVIEW,
        },
      },
    });

    expect(
      toSheetItem(base, {
        ...emptyCtx(),
        acknowledgedAgentErrors: { "session-1": "error-5" },
      }).agentErrorMessage,
    ).toBeNull();
    expect(
      toSheetItem(base, {
        ...emptyCtx(),
        dismissedAgentErrors: { "session-1": "older-error" },
      }).agentErrorMessage,
    ).toBe(ERROR_PREVIEW);
  });
});
