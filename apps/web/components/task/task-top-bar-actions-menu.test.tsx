import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { AppState } from "@/lib/state/store";
import { TaskTopBarActionsMenu } from "./task-top-bar-actions-menu";
import type { TaskActionsMenuBoardRow } from "@/hooks/use-task-actions-menu";

const mockClient = { subscribe: vi.fn(() => vi.fn()) };
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => mockClient,
}));

afterEach(() => {
  cleanup();
  mockClient.subscribe.mockClear();
});

const TASK_ID = "task-1";
const OLD_WORKFLOW_ID = "wf-old";
const NEW_WORKFLOW_ID = "wf-new";
const TRIGGER_TEST_ID = "task-topbar-actions-menu";
const MOVE_TO_TEST_ID = "task-context-move-to";

function initialStateWithTaskInActiveKanban(): Partial<AppState> {
  return {
    kanban: {
      workflowId: OLD_WORKFLOW_ID,
      tasks: [
        {
          id: TASK_ID,
          workflowId: OLD_WORKFLOW_ID,
          workflowStepId: "step-a",
          title: "Task 1",
          position: 0,
        },
      ],
    },
  } as never;
}

function initialStateWithMoveTargets(): Partial<AppState> {
  return {
    kanban: {
      workflowId: OLD_WORKFLOW_ID,
      tasks: [
        {
          id: TASK_ID,
          workflowId: OLD_WORKFLOW_ID,
          workflowStepId: "step-a",
          title: "Task 1",
          position: 0,
        },
      ],
    },
    kanbanMulti: {
      snapshots: {
        [OLD_WORKFLOW_ID]: {
          workflowId: OLD_WORKFLOW_ID,
          workflowName: "Workflow",
          steps: [
            { id: "step-a", title: "Step A", position: 0 },
            { id: "step-b", title: "Step B", position: 1 },
          ],
          tasks: [
            {
              id: TASK_ID,
              workflowId: OLD_WORKFLOW_ID,
              workflowStepId: "step-a",
              title: "Task 1",
              position: 0,
            },
          ],
        },
      },
    },
    workflows: {
      items: [{ id: OLD_WORKFLOW_ID, name: "Workflow", workspaceId: "ws-1", hidden: false }],
    },
  } as never;
}

const RESOLVED_BOARD_ROW: TaskActionsMenuBoardRow = {
  id: TASK_ID,
  title: "Task 1",
  workflowStepId: "step-a",
};

let capturedStore: StoreApi<AppState> | null = null;

function StoreCapture() {
  capturedStore = useAppStoreApi();
  return null;
}

function renderMenu(initialState: Partial<AppState>) {
  capturedStore = null;
  render(
    <ToastProvider>
      <StateProvider initialState={initialState as never}>
        <StoreCapture />
        <TaskTopBarActionsMenu
          taskId={TASK_ID}
          taskTitle="Task 1"
          boardRow={null}
          workspaceId="ws-1"
        />
      </StateProvider>
    </ToastProvider>,
  );
  return () => capturedStore!;
}

function renderMenuWithBoardRow(
  initialState: Partial<AppState>,
  boardRow: TaskActionsMenuBoardRow | null,
) {
  capturedStore = null;
  const { rerender } = render(
    <ToastProvider>
      <StateProvider initialState={initialState as never}>
        <StoreCapture />
        <TaskTopBarActionsMenu
          taskId={TASK_ID}
          taskTitle="Task 1"
          boardRow={boardRow}
          workspaceId="ws-1"
        />
      </StateProvider>
    </ToastProvider>,
  );
  return {
    getStore: () => capturedStore!,
    setBoardRow: (next: TaskActionsMenuBoardRow | null) =>
      rerender(
        <ToastProvider>
          <StateProvider initialState={initialState as never}>
            <StoreCapture />
            <TaskTopBarActionsMenu
              taskId={TASK_ID}
              taskTitle="Task 1"
              boardRow={next}
              workspaceId="ws-1"
            />
          </StateProvider>
        </ToastProvider>,
      ),
  };
}

function openMenu(trigger: HTMLElement) {
  act(() => {
    trigger.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true }));
    trigger.click();
  });
}

function expectMenuOpen(trigger: HTMLElement, open: boolean) {
  expect(trigger.getAttribute("aria-expanded")).toBe(String(open));
}

describe("TaskTopBarActionsMenu — WS subscription lifetime", () => {
  it("subscribes to the subject task's own WS updates for the lifetime of the trigger", () => {
    const { unmount } = render(
      <ToastProvider>
        <StateProvider initialState={initialStateWithTaskInActiveKanban() as never}>
          <TaskTopBarActionsMenu
            taskId={TASK_ID}
            taskTitle="Task 1"
            boardRow={null}
            workspaceId="ws-1"
          />
        </StateProvider>
      </ToastProvider>,
    );

    expect(mockClient.subscribe).toHaveBeenCalledWith(TASK_ID);
    const unsubscribe = mockClient.subscribe.mock.results[0]?.value as ReturnType<typeof vi.fn>;

    unmount();

    expect(unsubscribe).toHaveBeenCalledTimes(1);
  });

  it("keeps the trigger mounted when a same-message workflow move lands the task in a workflow with no prior snapshot", () => {
    const getStore = renderMenu(initialStateWithTaskInActiveKanban());
    expect(screen.getByTestId(TRIGGER_TEST_ID)).toBeTruthy();

    // A single task.updated for a cross-workflow move removes the task from
    // the old workflow's collections and adds it to the new one's snapshot
    // in one state transition (mirroring applyTaskUpdatedCache), so the
    // subject is never simultaneously absent from both.
    act(() => {
      getStore().setState((state) => ({
        kanban: { ...state.kanban, tasks: [] },
        kanbanMulti: {
          ...state.kanbanMulti,
          snapshots: {
            ...state.kanbanMulti.snapshots,
            [NEW_WORKFLOW_ID]: {
              workflowId: NEW_WORKFLOW_ID,
              workflowName: "New workflow",
              steps: [],
              tasks: [
                {
                  id: TASK_ID,
                  workflowId: NEW_WORKFLOW_ID,
                  workflowStepId: "step-b",
                  title: "Task 1",
                  position: 0,
                },
              ],
            },
          },
        },
      }));
    });

    expect(screen.getByTestId(TRIGGER_TEST_ID)).toBeTruthy();
  });
});

describe("TaskTopBarActionsMenu — live demotion on board-row loss (AC-TASKS-TASK-ACTIONS-MENU-002.6/004.1c)", () => {
  it("demotes Move to out of the menu in place, keeping the top-level menu open, when the board row is lost while the menu is open", () => {
    const { setBoardRow } = renderMenuWithBoardRow(
      initialStateWithMoveTargets(),
      RESOLVED_BOARD_ROW,
    );
    const trigger = screen.getByTestId(TRIGGER_TEST_ID);

    openMenu(trigger);
    expectMenuOpen(trigger, true);
    expect(screen.getByTestId(MOVE_TO_TEST_ID)).toBeTruthy();

    act(() => {
      setBoardRow(null);
    });

    expectMenuOpen(trigger, true);
    expect(screen.queryByTestId(MOVE_TO_TEST_ID)).toBeNull();
  });

  it("re-promotes Move to back into the menu in place when the board row resolves again while the menu stays open", () => {
    const { setBoardRow } = renderMenuWithBoardRow(initialStateWithMoveTargets(), null);
    const trigger = screen.getByTestId(TRIGGER_TEST_ID);

    openMenu(trigger);
    expectMenuOpen(trigger, true);
    expect(screen.queryByTestId(MOVE_TO_TEST_ID)).toBeNull();

    act(() => {
      setBoardRow(RESOLVED_BOARD_ROW);
    });

    expectMenuOpen(trigger, true);
    expect(screen.getByTestId(MOVE_TO_TEST_ID)).toBeTruthy();
  });
});

describe("TaskTopBarActionsMenu — closes on genuine removal (AC-TASKS-TASK-ACTIONS-MENU-004.5)", () => {
  it("closes the trigger's open menu once the subject leaves every board collection for good", () => {
    const getStore = renderMenu(initialStateWithTaskInActiveKanban());
    const trigger = screen.getByTestId(TRIGGER_TEST_ID);

    openMenu(trigger);
    expectMenuOpen(trigger, true);

    act(() => {
      getStore().setState((state) => ({
        kanban: { ...state.kanban, tasks: [] },
      }));
    });

    expectMenuOpen(trigger, false);
  });
});

describe("TaskTopBarActionsMenu — Escape closes and returns focus (AC-TASKS-TASK-ACTIONS-MENU-001.9)", () => {
  it("closes the menu and returns focus to the trigger on Escape", async () => {
    renderMenu(initialStateWithTaskInActiveKanban());
    const trigger = screen.getByTestId(TRIGGER_TEST_ID);

    openMenu(trigger);
    expectMenuOpen(trigger, true);

    const menu = await screen.findByRole("menu");
    act(() => {
      menu.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    });

    expectMenuOpen(trigger, false);
    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });
});
