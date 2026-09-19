import { describe, expect, it } from "vitest";
import { buildSidebarItem } from "./task-session-sidebar-item";

type SidebarTask = Parameters<typeof buildSidebarItem>[0];
type SidebarContext = Parameters<typeof buildSidebarItem>[1];

function emptyContext(): SidebarContext {
  return {
    repositorySlugById: new Map(),
    titleById: new Map(),
    workflowNameById: new Map(),
    stepTitleById: new Map(),
  };
}

function task(overrides: Partial<SidebarTask> = {}): SidebarTask {
  return {
    id: "t1",
    _workflowId: "wf1",
    title: "Task",
    workflowStepId: "step-1",
    ...overrides,
  } as SidebarTask;
}

describe("buildSidebarItem pending archive projection", () => {
  it("marks active rows covered by a pending archive", () => {
    const item = buildSidebarItem(task(), {
      ...emptyContext(),
      pendingArchiveTaskIds: new Set(["t1"]),
    });

    expect(item.isPendingArchive).toBe(true);
  });

  it("does not mark confirmed archived rows as pending", () => {
    const item = buildSidebarItem(task({ isArchived: true }), {
      ...emptyContext(),
      pendingArchiveTaskIds: new Set(["t1"]),
    });

    expect(item.isPendingArchive).toBe(false);
  });
});
