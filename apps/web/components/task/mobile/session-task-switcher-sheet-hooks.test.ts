import { describe, expect, it } from "vitest";
import { toSheetItem } from "./session-task-switcher-sheet-hooks";

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
});
