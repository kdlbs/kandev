import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TaskItemWithContextMenu } from "./task-switcher-context-menu";
import type { TaskSwitcherItem } from "./task-switcher-types";

const WORKFLOW_ID = "workflow-1";
const STEP_ID = "step-1";

function task(overrides: Partial<TaskSwitcherItem> = {}): TaskSwitcherItem {
  return { id: "task-1", title: "Task 1", state: "IN_PROGRESS", ...overrides };
}

function storeTask(item: TaskSwitcherItem) {
  return {
    id: item.id,
    title: item.title,
    workflowId: item.workflowId,
    workflowStepId: item.workflowStepId,
    position: 0,
  };
}

afterEach(cleanup);

describe("TaskItemWithContextMenu Nest under candidates", () => {
  // @covers AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.1
  // @covers AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.2
  it("uses rendered candidates when the workflow snapshot is partial", async () => {
    const current = task({ workflowId: WORKFLOW_ID, workflowStepId: STEP_ID });
    const target = task({
      id: "visible-target",
      title: "Visible target",
      workflowId: WORKFLOW_ID,
      workflowStepId: STEP_ID,
    });

    render(
      <StateProvider
        initialState={
          {
            kanban: {
              workflowId: WORKFLOW_ID,
              steps: [],
              tasks: [storeTask(current), storeTask(target)],
            },
            kanbanMulti: {
              isLoading: false,
              snapshots: {
                [WORKFLOW_ID]: {
                  workflowId: WORKFLOW_ID,
                  workflowName: "Workflow 1",
                  steps: [],
                  tasks: [storeTask(current)],
                  isPlaceholder: true,
                },
              },
            },
          } as never
        }
      >
        <ToastProvider>
          <TaskItemWithContextMenu task={current} nestCandidateTasks={[current, target]}>
            <div data-testid="task-row">Task 1</div>
          </TaskItemWithContextMenu>
        </ToastProvider>
      </StateProvider>,
    );

    fireEvent.contextMenu(screen.getByTestId("task-row"));
    const nestUnder = await screen.findByRole("menuitem", { name: "Nest under" });
    fireEvent.pointerMove(nestUnder, { pointerType: "mouse" });

    expect(await screen.findByRole("menuitem", { name: "Visible target" })).not.toBeNull();
  });

  it("captures the latest candidates when the menu opens", async () => {
    const current = task({ workflowId: WORKFLOW_ID, workflowStepId: STEP_ID });
    const staleTarget = task({
      id: "stale-target",
      title: "Stale target",
      workflowId: WORKFLOW_ID,
      workflowStepId: STEP_ID,
    });
    const latestTarget = task({
      id: "latest-target",
      title: "Latest target",
      workflowId: WORKFLOW_ID,
      workflowStepId: STEP_ID,
    });
    let candidates = [current, staleTarget];

    render(
      <StateProvider>
        <ToastProvider>
          <TaskItemWithContextMenu task={current} getNestCandidateTasks={() => candidates}>
            <div data-testid="fresh-task-row">Task 1</div>
          </TaskItemWithContextMenu>
        </ToastProvider>
      </StateProvider>,
    );
    candidates = [current, latestTarget];

    fireEvent.contextMenu(screen.getByTestId("fresh-task-row"));
    const nestUnder = await screen.findByRole("menuitem", { name: "Nest under" });
    fireEvent.pointerMove(nestUnder, { pointerType: "mouse" });

    expect(await screen.findByRole("menuitem", { name: "Latest target" })).not.toBeNull();
    expect(screen.queryByRole("menuitem", { name: "Stale target" })).toBeNull();
  });
});
