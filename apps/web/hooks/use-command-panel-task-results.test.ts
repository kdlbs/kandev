import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const listTasksByWorkspace = vi.fn();
vi.mock("@/lib/api", () => ({
  listTasksByWorkspace: (...args: unknown[]) => listTasksByWorkspace(...args),
}));

import { useInlineTaskSearchEffect } from "./use-command-panel-task-results";
import type { Task } from "@/lib/types/http";
import { taskId, workflowId, workspaceId } from "@/lib/types/ids";

function task(id: string, title: string): Task {
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
  };
}

const STEPS = [{ id: "step-1", position: 0, show_in_command_panel: true }];

function harness(setTaskResults: (t: Task[]) => void, setIsSearching: (s: boolean) => void) {
  return renderHook(
    ({ mode }: { mode: "commands" | "search-tasks" }) =>
      useInlineTaskSearchEffect({
        mode,
        search: "",
        open: true,
        workspaceId: "workspace-1",
        steps: STEPS,
        setTaskResults,
        setIsSearching,
      }),
    { initialProps: { mode: "commands" } as { mode: "commands" | "search-tasks" } },
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
});
