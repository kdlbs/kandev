import { describe, expect, it } from "vitest";

import { hasLoadedKanbanTaskSources, shouldCloseMissingSelectedTask } from "./kanban-with-preview";
import type { Task } from "./kanban-card";

const TASK: Task = {
  id: "task-1",
  title: "Task",
  workflowStepId: "step-1",
  state: "TODO",
  description: "",
  position: 0,
};

describe("shouldCloseMissingSelectedTask", () => {
  it("keeps a direct route task open while SPA task sources are still loading", () => {
    expect(
      shouldCloseMissingSelectedTask({
        isOpen: true,
        selectedTaskId: "task-1",
        selectedTask: null,
        initialTaskId: "task-1",
        kanbanIsLoading: false,
        hasLoadedTaskSources: false,
      }),
    ).toBe(false);

    expect(
      shouldCloseMissingSelectedTask({
        isOpen: true,
        selectedTaskId: "task-1",
        selectedTask: null,
        initialTaskId: "task-1",
        kanbanIsLoading: true,
        hasLoadedTaskSources: true,
      }),
    ).toBe(false);
  });

  it("closes a missing selected task once task sources have loaded", () => {
    expect(
      shouldCloseMissingSelectedTask({
        isOpen: true,
        selectedTaskId: "task-1",
        selectedTask: null,
        initialTaskId: "task-1",
        kanbanIsLoading: false,
        hasLoadedTaskSources: true,
      }),
    ).toBe(true);
  });

  it("closes a missing selected task that is not the direct route task", () => {
    expect(
      shouldCloseMissingSelectedTask({
        isOpen: true,
        selectedTaskId: "task-2",
        selectedTask: null,
        initialTaskId: "task-1",
        kanbanIsLoading: false,
        hasLoadedTaskSources: false,
      }),
    ).toBe(true);
  });

  it("does not close when the selected task is present", () => {
    expect(
      shouldCloseMissingSelectedTask({
        isOpen: true,
        selectedTaskId: "task-1",
        selectedTask: TASK,
        initialTaskId: "task-1",
        kanbanIsLoading: false,
        hasLoadedTaskSources: true,
      }),
    ).toBe(false);
  });
});

describe("hasLoadedKanbanTaskSources", () => {
  it("treats an active workflow as loaded even when it has no tasks", () => {
    expect(
      hasLoadedKanbanTaskSources({
        activeWorkflowId: "workflow-1",
        multiSnapshotCount: 0,
      }),
    ).toBe(true);
  });

  it("treats multi-workflow snapshots as loaded sources", () => {
    expect(
      hasLoadedKanbanTaskSources({
        activeWorkflowId: null,
        multiSnapshotCount: 1,
      }),
    ).toBe(true);
  });

  it("stays unloaded before any workflow or snapshot is available", () => {
    expect(
      hasLoadedKanbanTaskSources({
        activeWorkflowId: null,
        multiSnapshotCount: 0,
      }),
    ).toBe(false);
  });
});
