import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { Task } from "@/lib/types/http";
import { useTaskActionsMenuBoardRow } from "./task-page-content-helpers";

const TASK_ID = "task-1";

function makeTask(overrides: Partial<Task> = {}): Task {
  return {
    id: TASK_ID,
    title: "Fix the sidebar",
    workspace_id: "ws-1",
    workflow_id: "wf-1",
    workflow_step_id: "step-1",
    priority: "medium",
    repositories: [],
    created_at: "",
    updated_at: "",
    ...overrides,
  } as Task;
}

describe("useTaskActionsMenuBoardRow (AC-TASKS-TASK-ACTIONS-MENU-002.5)", () => {
  it("resolves the board row from a workflow snapshot, not the page's own task record", () => {
    function wrapper({ children }: { children: React.ReactNode }) {
      return (
        <StateProvider
          initialState={
            {
              kanbanMulti: {
                snapshots: {
                  "wf-1": {
                    workflowId: "wf-1",
                    workflowName: "Workflow 1",
                    steps: [],
                    tasks: [
                      {
                        id: TASK_ID,
                        workflowId: "wf-1",
                        workflowStepId: "step-1",
                        title: "Fix the sidebar (live)",
                        position: 0,
                        parentTaskId: "task-parent",
                      },
                    ],
                  },
                },
                isLoading: false,
              },
            } as never
          }
        >
          {children}
        </StateProvider>
      );
    }

    const { result } = renderHook(() => useTaskActionsMenuBoardRow(makeTask()), { wrapper });

    expect(result.current).toEqual(
      expect.objectContaining({
        id: TASK_ID,
        title: "Fix the sidebar (live)",
        workflowStepId: "step-1",
        parentTaskId: "task-parent",
      }),
    );
  });

  it("returns null (the identifier-only tier) for a task with no board row anywhere on the board", () => {
    function wrapper({ children }: { children: React.ReactNode }) {
      return <StateProvider>{children}</StateProvider>;
    }

    const { result } = renderHook(() => useTaskActionsMenuBoardRow(makeTask()), { wrapper });

    expect(result.current).toBeNull();
  });

  it("returns null when there is no subject task at all", () => {
    function wrapper({ children }: { children: React.ReactNode }) {
      return <StateProvider>{children}</StateProvider>;
    }

    const { result } = renderHook(() => useTaskActionsMenuBoardRow(null), { wrapper });

    expect(result.current).toBeNull();
  });
});
