import { describe, expect, it } from "vitest";
import type { KanbanState } from "@/lib/state/slices";
import { buildSidebarTaskEditTarget } from "./task-session-sidebar-edit";

function sourceTask(overrides: Partial<KanbanState["tasks"][number]> = {}) {
  return {
    id: "task-1",
    title: "Authoritative title",
    description: "Authoritative description",
    workflowStepId: "step-source",
    state: "IN_PROGRESS" as const,
    repositoryId: "repo-1",
    ...overrides,
  } as KanbanState["tasks"][number];
}

describe("buildSidebarTaskEditTarget", () => {
  it("combines clicked workflow metadata with authoritative task fields", () => {
    expect(
      buildSidebarTaskEditTarget(
        { id: "task-1", workflowId: "workflow-clicked", workflowStepId: "step-item" },
        sourceTask(),
      ),
    ).toEqual({
      id: "task-1",
      title: "Authoritative title",
      description: "Authoritative description",
      workflowId: "workflow-clicked",
      workflowStepId: "step-source",
      state: "IN_PROGRESS",
      repositoryId: "repo-1",
    });
  });

  it("returns no target when the item has no workflow", () => {
    expect(buildSidebarTaskEditTarget({ id: "task-1" }, sourceTask())).toBeNull();
  });

  it("returns no target when the source task cannot be resolved", () => {
    expect(
      buildSidebarTaskEditTarget(
        { id: "task-1", workflowId: "workflow-1", workflowStepId: "step-1" },
        null,
      ),
    ).toBeNull();
  });

  it("falls back to the sidebar step when the source task has no step", () => {
    expect(
      buildSidebarTaskEditTarget(
        { id: "task-1", workflowId: "workflow-1", workflowStepId: "step-item" },
        sourceTask({ workflowStepId: undefined }),
      )?.workflowStepId,
    ).toBe("step-item");
  });
});
