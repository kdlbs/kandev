import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TaskItemWithContextMenu } from "./task-switcher-context-menu";
import type { TaskSwitcherItem } from "./task-switcher-types";

const WORKFLOW_ID = "workflow-1";
const STEP_ID = "step-1";
const workflowMoveMock = vi.hoisted(() => ({ move: vi.fn(), isMoving: false }));
const touchDrawerMock = vi.hoisted(() => ({ enabled: false }));

vi.mock("@/hooks/domains/kanban/use-workflow-move", () => ({
  useWorkflowMove: () => workflowMoveMock,
}));
vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => touchDrawerMock.enabled,
}));

afterEach(() => {
  cleanup();
  Object.defineProperty(window, "innerWidth", { configurable: true, value: 1024 });
});

function task(overrides: Partial<TaskSwitcherItem> = {}): TaskSwitcherItem {
  return { id: "task-1", title: "Task 1", state: "IN_PROGRESS", ...overrides };
}

describe("TaskItemWithContextMenu for Office-owned tasks", () => {
  it("hides workflow actions", async () => {
    render(
      <StateProvider>
        <ToastProvider>
          <TaskItemWithContextMenu
            task={task({
              workflowId: WORKFLOW_ID,
              workflowStepId: STEP_ID,
              isFromOffice: true,
            })}
            workflows={[{ id: "workflow-2", name: "Workflow 2" }]}
            stepsByWorkflowId={{
              [WORKFLOW_ID]: [
                { id: STEP_ID, title: "Step 1" },
                { id: "step-2", title: "Step 2" },
              ],
            }}
          >
            <div data-testid="office-task-row">Office task</div>
          </TaskItemWithContextMenu>
        </ToastProvider>
      </StateProvider>,
    );

    fireEvent.contextMenu(screen.getByTestId("office-task-row"));
    const menu = await screen.findByRole("menu");
    expect(within(menu).queryByRole("menuitem", { name: "Move to" })).toBeNull();
    expect(within(menu).queryByRole("menuitem", { name: "Change workflow..." })).toBeNull();
  });
});
