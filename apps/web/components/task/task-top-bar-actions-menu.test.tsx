import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { AppState } from "@/lib/state/store";
import { TaskTopBarActionsMenu } from "./task-top-bar-actions-menu";

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

function openMenu(trigger: HTMLElement) {
  act(() => {
    trigger.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true }));
    trigger.click();
  });
}

function expectMenuOpen(trigger: HTMLElement, open: boolean) {
  expect(trigger.getAttribute("aria-expanded")).toBe(String(open));
}

describe("TaskTopBarActionsMenu — live subscription for board-row demotion (AC-TASKS-TASK-ACTIONS-MENU-002.6/004.1c)", () => {
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
