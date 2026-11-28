import { act, cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { MoveTaskResponse, TaskSession } from "@/lib/types/http";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { StateHydrator } from "@/components/state-hydrator";
import { captureTaskNavigationSelection } from "@/lib/state/task-navigation-selection";
import { taskRouteSelectionState } from "@/src/task-detail-route.test-helpers";
import { useWorkflowStepMove } from "./use-workflow-step-move";

const network = vi.hoisted(() => ({ moveTask: vi.fn() }));
vi.mock("@/lib/api", async (original) => ({
  ...(await original<typeof import("@/lib/api")>()),
  ...network,
}));
const WORKFLOW_ID = "workflow-1";
let store: StoreApi<AppState>;
let move: ReturnType<typeof useWorkflowStepMove>;
function Harness() {
  store = useAppStoreApi();
  move = useWorkflowStepMove({
    taskId: "task-1",
    workflowId: WORKFLOW_ID,
    currentStepId: "source",
    presentationToken: 1,
  });
  return null;
}
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.17
it.each([
  [true, false],
  [true, true],
  [false, false],
])(
  "settles deferred route with no-focus=%s and newer unrelated task projection=%s",
  async (noFocus, newerProjection) => {
    const sessions = ["source", "requested"].map((id) => ({
      id,
      task_id: "task-1",
      state: "COMPLETED",
      started_at: "2026-11-20T12:00:00Z",
    })) as TaskSession[];
    const state = taskRouteSelectionState("task-1", "source", sessions);
    const response = Promise.withResolvers<MoveTaskResponse>();
    network.moveTask.mockReturnValue(response.promise);
    const page = render(
      <StateProvider initialState={state}>
        <Harness />
      </StateProvider>,
    );
    const owner = captureTaskNavigationSelection(store);
    let pending!: Promise<boolean>;
    act(() => {
      pending = move.handleMove("agentless");
    });
    page.rerender(
      <StateProvider initialState={{}}>
        <Harness />
        <StateHydrator
          requestedSessionId="requested"
          taskSelectionAtRequestStart={owner}
          initialState={{
            tasks: { ...store.getState().tasks, activeSessionId: "requested" },
            taskSessionsByTask: {
              ...store.getState().taskSessionsByTask,
              itemsByTaskId: { "task-1": sessions },
              loadedByTaskId: { "task-1": true },
            },
          }}
        />
      </StateProvider>,
    );
    expect(store.getState().tasks.activeSessionId).toBe("source");
    if (newerProjection)
      act(() =>
        store.setState((current) => ({
          kanban: {
            ...current.kanban,
            tasks: [
              {
                id: "task-1",
                workflowId: WORKFLOW_ID,
                workflowStepId: "agentless",
                title: "Newer title, same entry",
                updatedAt: "2026-11-20T12:02:00Z",
                metadata: {},
              },
            ] as never,
          },
        })),
      );
    await act(async () => {
      response.resolve({
        workflow_entry_identity: "entry:00000000000000000042",
        ...(noFocus ? { workflow_focus_disposition: "none" as const } : {}),
        task: {
          id: "task-1",
          workflow_id: WORKFLOW_ID,
          workflow_step_id: "agentless",
          metadata: {},
          updated_at: "2026-11-20T12:01:00Z",
        },
        workflow_step: { id: "agentless", workflow_id: WORKFLOW_ID, events: {} },
      } as MoveTaskResponse);
      expect(await pending).toBe(true);
    });
    if (noFocus) {
      await waitFor(() => expect(store.getState().tasks.activeSessionId).toBe("requested"));
      expect(store.getState().workflowSessionFocus.intent).toBeNull();
    } else {
      expect(store.getState().tasks.activeSessionId).toBe("source");
      expect(store.getState().workflowSessionFocus.intent).not.toBeNull();
      act(() =>
        store.getState().reconcileWorkflowSessionFocus("task-1", {
          workflowStepId: "agentless",
          entryIdentity: "entry:00000000000000000042",
          updatedAt: "2026-11-20T12:02:00Z",
          metadata: {
            workflow_session_route: {
              operation_id: "move",
              entry_identity: "entry:00000000000000000042",
              destination_step_id: "agentless",
              destination_session_id: "source",
              phase: "committed",
            },
          },
        }),
      );
      expect(store.getState().tasks.activeSessionId).toBe("source");
      expect(store.getState().workflowSessionFocus.request?.sessionId).toBe("source");
    }
  },
);
