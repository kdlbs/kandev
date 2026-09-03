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
