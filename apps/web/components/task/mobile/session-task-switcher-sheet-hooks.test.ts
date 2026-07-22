import { describe, expect, it } from "vitest";
import { toSheetItem } from "./session-task-switcher-sheet-hooks";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type TaskSession,
} from "@/lib/types/http";

type SheetTask = Parameters<typeof toSheetItem>[0];
type SheetCtx = Parameters<typeof toSheetItem>[1];

function emptyCtx(): SheetCtx {
  return {
    repositoryPathsById: new Map(),
    workflowNameById: new Map(),
    stepTitleById: new Map(),
    sessionsById: {},
    sessionsByTaskId: {},
    gitStatusByEnvId: {},
    envIdBySessionId: {},
    messagesBySession: {},
    dismissedAgentErrors: {},
    acknowledgedAgentErrors: {},
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

function session(id: string, state: TaskSession["state"]): TaskSession {
  return {
    id: toSessionId(id),
    task_id: toTaskId("t1"),
    state,
    started_at: "2026-07-22T00:00:00Z",
    updated_at: "2026-07-22T00:00:00Z",
  };
}

function pendingPermission(id: string, sessionId: string): Message {
  return {
    id,
    session_id: toSessionId(sessionId),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content: "Allow?",
    type: "permission_request",
    metadata: { status: "pending" },
    created_at: "2026-07-22T00:00:00Z",
  };
}

describe("toSheetItem", () => {
  // The mobile task-switcher row must read the same task-level most-active-wins
  // aggregate the desktop sidebar and board card read, so a background-running
  // secondary session is caught on mobile too (§spec:task-level-truth).
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

  it("finds pending permission in a secondary waiting session", () => {
    const ctx = emptyCtx();
    ctx.sessionsByTaskId.t1 = [
      session("primary", "STARTING"),
      session("secondary", "WAITING_FOR_INPUT"),
    ];
    ctx.messagesBySession.primary = [];
    ctx.messagesBySession.secondary = [pendingPermission("permission", "secondary")];

    const item = toSheetItem(task({ primarySessionId: "primary" }), ctx);

    expect(item.hasPendingPermission).toBe(true);
    expect(item.hasPendingClarification).toBe(false);
  });

  it("excludes stale pending permission from a starting session", () => {
    const ctx = emptyCtx();
    ctx.sessionsByTaskId.t1 = [session("starting", "STARTING")];
    ctx.messagesBySession.starting = [pendingPermission("stale-permission", "starting")];

    const item = toSheetItem(task({ primarySessionId: "starting" }), ctx);

    expect(item.hasPendingPermission).toBe(false);
    expect(item.hasPendingClarification).toBe(false);
  });
});
