import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const listTasksByWorkspace = vi.fn();
vi.mock("@/lib/api", () => ({
  listTasksByWorkspace: (...args: unknown[]) => listTasksByWorkspace(...args),
}));

import { useInlineTaskSearchEffect } from "./use-command-panel-task-results";
import type { Task } from "@/lib/types/http";
import { taskId, workflowId, workspaceId } from "@/lib/types/ids";

function task(id: string, title: string, overrides: Partial<Task> = {}): Task {
  return {
    id: taskId(id),
    workspace_id: workspaceId("workspace-1"),
    workflow_id: workflowId("workflow-1"),
    workflow_step_id: "step-1",
    position: 0,
    title,
    description: "",
    state: "IN_PROGRESS",
    priority: "medium",
    created_at: "2026-08-24T09:00:00Z",
    updated_at: "2026-08-24T09:00:00Z",
    ...overrides,
  };
}

const STEPS = [{ id: "step-1", position: 0, show_in_command_panel: true }];

type HarnessProps = {
  mode: "commands" | "search-tasks";
  search?: string;
};

function harness(
  setTaskResults: (t: Task[]) => void,
  setIsSearching: (s: boolean) => void,
  search = "",
) {
  return renderHook(
    ({ mode, search = "" }: HarnessProps) =>
      useInlineTaskSearchEffect({
        mode,
        search,
        open: true,
        workspaceId: "workspace-1",
        steps: STEPS,
        setTaskResults,
        setIsSearching,
      }),
    {
      initialProps: { mode: "commands", search } as HarnessProps,
    },
  );
}

afterEach(() => {
  listTasksByWorkspace.mockReset();
});

describe("useInlineTaskSearchEffect", () => {
  it("caps the commands scope at a five-row preview and the tasks scope at twenty", async () => {
    const many = Array.from({ length: 20 }, (_, i) => task(`task-${i}`, `Task ${i}`));
    listTasksByWorkspace.mockResolvedValue({ tasks: many });
    const setTaskResults = vi.fn();
    const { rerender } = harness(setTaskResults, vi.fn());

    await waitFor(() => expect(setTaskResults).toHaveBeenCalledWith(many.slice(0, 5)));

    setTaskResults.mockClear();
    rerender({ mode: "search-tasks" });
    await waitFor(() => expect(setTaskResults).toHaveBeenCalledWith(many));
  });

  it("drops the previous scope's results when the scope changes", async () => {
    listTasksByWorkspace.mockResolvedValue({ tasks: [task("task-1", "Only task")] });
    const setTaskResults = vi.fn();
    const { rerender } = harness(setTaskResults, vi.fn());
    await waitFor(() => expect(setTaskResults).toHaveBeenCalled());

    setTaskResults.mockClear();
    rerender({ mode: "search-tasks" });

    // The five-row preview must not stand in as the tasks scope's full result
    // set (nor suppress its loading state) while the wider request is in flight.
    expect(setTaskResults).toHaveBeenNthCalledWith(1, []);
  });

  it("keeps results when the scope is unchanged", async () => {
    listTasksByWorkspace.mockResolvedValue({ tasks: [task("task-1", "Only task")] });
    const setTaskResults = vi.fn();
    const { rerender } = harness(setTaskResults, vi.fn());
    await waitFor(() => expect(setTaskResults).toHaveBeenCalled());

    setTaskResults.mockClear();
    rerender({ mode: "commands" });
    expect(setTaskResults).not.toHaveBeenCalledWith([]);
  });

  // @covers AC-TASKS-COMMAND-PANEL-ARCHIVED-TASKS-001.5
  it("uses archived_at instead of terminal state for the active task preview", async () => {
    const archivedInProgress = task("archived-in-progress", "Archived in progress", {
      state: "IN_PROGRESS",
      archived_at: "2026-08-24T09:05:00Z",
    });
    const unarchivedCompleted = task("unarchived-completed", "Unarchived completed", {
      state: "COMPLETED",
    });
    listTasksByWorkspace.mockResolvedValue({ tasks: [archivedInProgress, unarchivedCompleted] });
    const setTaskResults = vi.fn();

    harness(setTaskResults, vi.fn());

    await waitFor(() => expect(setTaskResults).toHaveBeenCalledWith([unarchivedCompleted]));
  });

  // @covers AC-TASKS-COMMAND-PANEL-ARCHIVED-TASKS-001.6
  it("ranks archived matches after non-archived matches by archived_at", async () => {
    const archivedInProgress = task("archived-in-progress", "Archived in progress", {
      state: "IN_PROGRESS",
      archived_at: "2026-08-24T09:05:00Z",
    });
    const unarchivedCompleted = task("unarchived-completed", "Unarchived completed", {
      state: "COMPLETED",
    });
    listTasksByWorkspace.mockResolvedValue({ tasks: [archivedInProgress, unarchivedCompleted] });
    const setTaskResults = vi.fn();

    harness(setTaskResults, vi.fn(), "archived");

    await waitFor(() =>
      expect(setTaskResults).toHaveBeenCalledWith([unarchivedCompleted, archivedInProgress]),
    );
  });

  it("queries active matches before a bounded archived fallback page", async () => {
    const archivedTasks = Array.from({ length: 20 }, (_, index) =>
      task(`archived-${index}`, `Archived ${index}`, {
        archived_at: `2026-08-24T09:${String(index).padStart(2, "0")}:00Z`,
      }),
    );
    const unarchivedTask = task("unarchived-match", "Unarchived match", {
      state: "COMPLETED",
    });
    listTasksByWorkspace.mockImplementation(
      (_workspaceId: string, params: { onlyArchived?: boolean }) =>
        Promise.resolve(
          params.onlyArchived
            ? { tasks: archivedTasks, total: 10_000 }
            : { tasks: [unarchivedTask], total: 1 },
        ),
    );
    const setTaskResults = vi.fn();

    harness(setTaskResults, vi.fn(), "match");

    await waitFor(() =>
      expect(setTaskResults).toHaveBeenCalledWith([unarchivedTask, ...archivedTasks.slice(0, 4)]),
    );
    expect(listTasksByWorkspace).toHaveBeenCalledTimes(2);
    expect(listTasksByWorkspace.mock.calls[0][1]).toEqual({
      query: "match",
      page: 1,
      pageSize: 5,
    });
    expect(listTasksByWorkspace.mock.calls[1][1]).toEqual({
      query: "match",
      page: 1,
      pageSize: 4,
      onlyArchived: true,
    });
  });
});
