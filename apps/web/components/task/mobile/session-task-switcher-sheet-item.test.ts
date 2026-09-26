import { describe, expect, it } from "vitest";
import { toSheetItem } from "./session-task-switcher-sheet-item";

type SheetTask = Parameters<typeof toSheetItem>[0];
type SheetCtx = Parameters<typeof toSheetItem>[1];

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

function emptyCtx(): SheetCtx {
  return {
    repositoryPathsById: new Map([
      ["repo-a", "owner/repo-a"],
      ["repo-b", "owner/repo-b"],
    ]),
    workflowNameById: new Map(),
    stepTitleById: new Map(),
  };
}

describe("toSheetItem repository projection", () => {
  it("projects unique repository slugs in attachment order", () => {
    const item = toSheetItem(
      task({
        repositoryId: "repo-a",
        repositories: [
          { id: "link-b", repository_id: "repo-b", base_branch: "main", position: 2 },
          { id: "link-a", repository_id: "repo-a", base_branch: "main", position: 1 },
          {
            id: "link-a-duplicate",
            repository_id: "repo-a",
            base_branch: "main",
            position: 3,
          },
        ],
      }),
      emptyCtx(),
    );

    expect(item.repositories).toEqual(["owner/repo-a", "owner/repo-b"]);
    expect(item.repositoryLinks).toHaveLength(3);
  });

  it("carries task priority into the phone task drawer row", () => {
    const item = toSheetItem(task({ priority: "critical" }), emptyCtx());

    expect(item.priority).toBe("critical");
  });

  it("marks active rows covered by a pending archive", () => {
    const item = toSheetItem(task(), {
      ...emptyCtx(),
      pendingArchiveTaskIds: new Set(["t1"]),
    });

    expect(item.isPendingArchive).toBe(true);
  });

  it("does not mark confirmed archived rows as pending", () => {
    const item = toSheetItem(task({ isArchived: true }), {
      ...emptyCtx(),
      pendingArchiveTaskIds: new Set(["t1"]),
    });

    expect(item.isPendingArchive).toBe(false);
  });
});

describe("toSheetItem activity projection", () => {
  it("preserves task activity and its task-local fallbacks on the phone row", () => {
    const item = toSheetItem(
      task({
        updatedAt: "task-updated",
        createdAt: "task-created",
        statusSummary: {
          revision: 3,
          updated_at: "summary-refreshed",
          last_activity_at: "task-activity",
        },
      }),
      emptyCtx(),
    );

    expect(item.lastActivityAt).toBe("task-activity");
    expect(item.updatedAt).toBe("summary-refreshed");
    expect(item.createdAt).toBe("task-created");
  });

  it("falls back to the task update time instead of summary freshness", () => {
    const item = toSheetItem(
      task({
        updatedAt: "task-updated",
        createdAt: "task-created",
        statusSummary: { revision: 3, updated_at: "summary-refreshed" },
      }),
      emptyCtx(),
    );

    expect(item.lastActivityAt).toBe("task-updated");
  });
});

describe("toSheetItem remote executor projection", () => {
  it("carries the exact remote executor scope onto the mobile sheet row", () => {
    const item = toSheetItem(
      task({
        isRemoteExecutor: true,
        primaryExecutorId: "executor-1",
        primaryExecutorType: "k8s",
        primaryExecutorName: "Cluster executor",
        primarySessionId: "session-1",
      }),
      emptyCtx(),
    );

    expect(item.remoteExecutorId).toBe("executor-1");
    expect(item.remoteExecutorType).toBe("k8s");
    expect(item.remoteExecutorName).toBe("Cluster executor");
    expect(item.primarySessionId).toBe("session-1");
  });
});

describe("toSheetItem Office identity projection", () => {
  // @covers AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.4
  it("carries Office identity into the phone task drawer row", () => {
    const item = toSheetItem(task({ isFromOffice: true }), emptyCtx());

    expect(item.isFromOffice).toBe(true);
  });
});
