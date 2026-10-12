import { describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { useTaskActionsMenuMoveTargets } from "./use-task-actions-menu-move-targets";

describe("useTaskActionsMenuMoveTargets — hidden step fallback (AC-TASKS-TASK-ACTIONS-MENU-002.3b)", () => {
  const WORKFLOW_ID = "wf-1";
  const TASK_ID = "task-1";
  const HIDDEN_STEP_ID = "step-hidden";

  function wrapper({ children }: { children: React.ReactNode }) {
    return (
      <StateProvider
        initialState={
          {
            kanbanMulti: {
              snapshots: {
                [WORKFLOW_ID]: {
                  workflowId: WORKFLOW_ID,
                  workflowName: "Workflow 1",
                  steps: [
                    { id: "step-a", title: "Todo", color: "blue", position: 0 },
                    { id: HIDDEN_STEP_ID, title: "Hidden step", color: "gray", position: 1 },
                    { id: "step-b", title: "Done", color: "green", position: 2 },
                  ],
                  tasks: [
                    {
                      id: TASK_ID,
                      workflowId: WORKFLOW_ID,
                      workflowStepId: "step-a",
                      title: "Task 1",
                      position: 0,
                    },
                  ],
                },
              },
              isLoading: false,
            },
            userSettings: {
              hiddenWorkflowStepIds: { [WORKFLOW_ID]: [HIDDEN_STEP_ID] },
            },
          } as never
        }
      >
        {children}
      </StateProvider>
    );
  }

  it("excludes hidden steps from the current workflow when the caller passes no explicit steps", () => {
    const { result } = renderHook(() => useTaskActionsMenuMoveTargets(TASK_ID), { wrapper });

    expect(result.current.currentWorkflowId).toBe(WORKFLOW_ID);
    const stepIds = result.current.stepsByWorkflowId[WORKFLOW_ID].map((step) => step.id);
    expect(stepIds).toEqual(["step-a", "step-b"]);
  });

  it("uses the caller's explicit steps as-is, bypassing the hidden-step fallback", () => {
    const explicitSteps = [{ id: HIDDEN_STEP_ID, title: "Hidden step", color: "gray" }];
    const { result } = renderHook(() => useTaskActionsMenuMoveTargets(TASK_ID, explicitSteps), {
      wrapper,
    });

    const stepIds = result.current.stepsByWorkflowId[WORKFLOW_ID].map((step) => step.id);
    expect(stepIds).toEqual([HIDDEN_STEP_ID]);
  });
});

describe("useTaskActionsMenuMoveTargets — flat-list fallback for a WS-arrived task (AC-TASKS-TASK-ACTIONS-MENU-002.1/002.2)", () => {
  const WORKFLOW_ID = "wf-1";
  const TASK_ID = "task-1";

  function wrapper({ children }: { children: React.ReactNode }) {
    return (
      <StateProvider
        initialState={
          {
            kanban: {
              tasks: [
                {
                  id: TASK_ID,
                  workflowId: WORKFLOW_ID,
                  workflowStepId: "step-a",
                  title: "Task 1",
                  position: 0,
                },
              ],
            },
            kanbanMulti: {
              // The workflow's own snapshot has already loaded (steps present),
              // but this task has not yet been merged into `snapshot.tasks` -
              // the exact WS-arrived-before-snapshot-catch-up race.
              snapshots: {
                [WORKFLOW_ID]: {
                  workflowId: WORKFLOW_ID,
                  workflowName: "Workflow 1",
                  steps: [
                    { id: "step-a", title: "Todo", color: "blue", position: 0 },
                    { id: "step-b", title: "Done", color: "green", position: 1 },
                  ],
                  tasks: [],
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

  it("resolves currentWorkflowId from the flat kanban.tasks list when no snapshot lists the task yet", () => {
    const { result } = renderHook(() => useTaskActionsMenuMoveTargets(TASK_ID), { wrapper });

    expect(result.current.currentWorkflowId).toBe(WORKFLOW_ID);
    const stepIds = result.current.stepsByWorkflowId[WORKFLOW_ID].map((step) => step.id);
    expect(stepIds).toEqual(["step-a", "step-b"]);
  });
});
