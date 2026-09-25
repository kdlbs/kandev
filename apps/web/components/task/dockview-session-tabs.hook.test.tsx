import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render } from "@testing-library/react";
import type { DockviewApi } from "dockview-react";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { TaskSession, TaskId } from "@/lib/types/http";
import { makeReorderingAutoSessionApi } from "./dockview-session-tabs.test-utils";
import { useAutoSessionTab } from "./dockview-session-tabs";

const TASK_ID = "task-1" as TaskId;
const ACTIVE_SESSION_ID = "session-a";
const SIBLING_SESSION_ID = "session-b";

function Harness() {
  useAutoSessionTab(ACTIVE_SESSION_ID);
  return null;
}

let capturedStore: StoreApi<AppState> | null = null;

function FocusHarness() {
  capturedStore = useAppStoreApi();
  useAutoSessionTab(ACTIVE_SESSION_ID);
  return null;
}

function renderHookWithHydratedSessions() {
  const sessions = [ACTIVE_SESSION_ID, SIBLING_SESSION_ID].map((id) => ({ id }) as TaskSession);

  return render(
    <StateProvider
      initialState={{
        ...defaultState,
        tasks: {
          ...defaultState.tasks,
          activeTaskId: TASK_ID,
          activeSessionId: ACTIVE_SESSION_ID,
        },
        taskSessionsByTask: {
          ...defaultState.taskSessionsByTask,
          itemsByTaskId: { [TASK_ID]: sessions },
        },
      }}
    >
      <Harness />
    </StateProvider>,
  );
}

afterEach(() => {
  cleanup();
  useDockviewStore.setState({ api: null });
});

describe("useAutoSessionTab", () => {
  it("reconciles every hydrated session when Dockview becomes ready later", () => {
    // @covers AC-UI-TASK-AGENT-TAB-RECONCILIATION-001.1
    useDockviewStore.setState({ api: null });
    renderHookWithHydratedSessions();

    const { api } = makeReorderingAutoSessionApi();
    act(() => {
      useDockviewStore.setState({ api: api as DockviewApi });
    });

    expect(api.panels.map((panel) => panel.id)).toEqual(
      expect.arrayContaining([`session:${ACTIVE_SESSION_ID}`, `session:${SIBLING_SESSION_ID}`]),
    );
  });

  it("subscribes to a same-session focus request and acknowledges after activation", () => {
    const { api, activationSequence, centerActivePanelId } = makeReorderingAutoSessionApi("files");
    const route = {
      operation_id: "workflow-session:task-1:step-b:entry:00000000000000000042:profile::reuse",
      destination_step_id: "step-b",
      entry_identity: "entry:00000000000000000042",
      destination_session_id: ACTIVE_SESSION_ID,
      phase: "committed",
    };
    capturedStore = null;

    render(
      <StateProvider
        initialState={{
          ...defaultState,
          tasks: {
            ...defaultState.tasks,
            activeTaskId: TASK_ID,
            activeSessionId: ACTIVE_SESSION_ID,
          },
          kanban: {
            ...defaultState.kanban,
            tasks: [
              {
                id: TASK_ID,
                workflowId: "workflow-1",
                workflowStepId: "step-b",
                title: "Task",
                position: 0,
                updatedAt: "2026-09-19T10:00:01Z",
                metadata: { workflow_session_route: route },
              },
            ],
          },
          taskSessionsByTask: {
            ...defaultState.taskSessionsByTask,
            itemsByTaskId: {
              [TASK_ID]: [
                { id: ACTIVE_SESSION_ID } as TaskSession,
                { id: SIBLING_SESSION_ID } as TaskSession,
              ],
            },
          },
        }}
      >
        <FocusHarness />
      </StateProvider>,
    );

    expect(capturedStore).not.toBeNull();
    const store = capturedStore as unknown as StoreApi<AppState>;
    act(() => {
      useDockviewStore.setState({ api: api as DockviewApi });
    });
    activationSequence.length = 0;

    const acknowledge = store.getState().acknowledgeWorkflowSessionFocus;
    const acknowledgeSpy = vi.fn((requestId: number) => acknowledge(requestId));
    store.setState({ acknowledgeWorkflowSessionFocus: acknowledgeSpy });

    act(() => {
      const requestId = store.getState().beginWorkflowSessionFocus({
        taskId: TASK_ID,
        workflowId: "workflow-1",
        destinationStepId: "step-b",
        presentationToken: 1,
        navigationRevision: 0,
      });
      expect(requestId).toBe(1);
      store.getState().bindWorkflowSessionFocus({
        requestId: requestId!,
        presentationToken: 1,
        entryIdentity: route.entry_identity,
      });
      store.getState().reconcileWorkflowSessionFocus(TASK_ID);
    });

    expect(centerActivePanelId()).toBe(`session:${ACTIVE_SESSION_ID}`);
    expect(acknowledgeSpy).toHaveBeenCalledTimes(1);
    expect(acknowledgeSpy).toHaveBeenCalledWith(1);
    expect(activationSequence.filter((id) => id === `session:${ACTIVE_SESSION_ID}`)).toHaveLength(
      1,
    );
    expect(store.getState().workflowSessionFocus.request).toBeNull();
  });
});
