import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render } from "@testing-library/react";
import type { DockviewApi } from "dockview-react";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi, useAppStore } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { LayoutState } from "@/lib/state/layout-manager";
import type { TaskSession, TaskId } from "@/lib/types/http";
import { makeReorderingAutoSessionApi } from "./dockview-session-tabs.test-utils";
import { useAutoSessionTab } from "./dockview-session-tabs";
import { setupSessionTabSync } from "./dockview-session-tab-sync";
import { StateHydrator } from "@/components/state-hydrator";
import { captureTaskNavigationSelection } from "@/lib/state/task-navigation-selection";
import { hiddenSessionIdsFor, hideSessionPanel } from "./dockview-hidden-session-panels";
import { captureTaskSessionHydrationEpochs } from "@/lib/state/slices/session/hydration-epochs";

const TASK_ID = "task-1" as TaskId;
const ACTIVE_SESSION_ID = "session-a";
const SIBLING_SESSION_ID = "session-b";

function Harness() {
  useAutoSessionTab(ACTIVE_SESSION_ID);
  return null;
}

let capturedStore: StoreApi<AppState> | null = null;
let stopTabSync = () => {};

function FocusHarness() {
  capturedStore = useAppStoreApi();
  useAutoSessionTab(useAppStore((state) => state.tasks.activeSessionId));
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
  stopTabSync();
  cleanup();
  useDockviewStore.setState({
    api: null,
    currentLayoutEnvId: null,
    preMaximizeLayout: null,
    maximizedGroupId: null,
    isRestoringLayout: false,
  });
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
});

describe("useAutoSessionTab maximize hydration", () => {
  it("reconciles saved maximize panels when session loading completes without changing ids", () => {
    const { api } = makeReorderingAutoSessionApi();
    const activePanelId = `session:${ACTIVE_SESSION_ID}`;
    const siblingPanelId = `session:${SIBLING_SESSION_ID}`;
    const stalePanelId = "session:saved-stale";
    const preMaximizeLayout = {
      columns: [
        {
          id: "center",
          groups: [
            {
              id: "center",
              panels: [
                {
                  id: activePanelId,
                  component: "chat",
                  title: "Agent",
                  params: { sessionId: ACTIVE_SESSION_ID },
                },
                {
                  id: siblingPanelId,
                  component: "chat",
                  title: "Sibling",
                  params: { sessionId: SIBLING_SESSION_ID },
                },
                {
                  id: stalePanelId,
                  component: "chat",
                  title: "Stale",
                  params: { sessionId: "saved-stale" },
                },
              ],
              activePanel: activePanelId,
            },
          ],
        },
      ],
    } satisfies LayoutState;
    useDockviewStore.setState({
      api: api as DockviewApi,
      currentLayoutEnvId: "env-a",
      preMaximizeLayout,
    });
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
          taskSessionsByTask: {
            ...defaultState.taskSessionsByTask,
            itemsByTaskId: {
              [TASK_ID]: [
                { id: ACTIVE_SESSION_ID } as TaskSession,
                { id: SIBLING_SESSION_ID } as TaskSession,
              ],
            },
            loadedByTaskId: { [TASK_ID]: false },
          },
        }}
      >
        <FocusHarness />
      </StateProvider>,
    );

    expect(capturedStore).not.toBeNull();
    const store = capturedStore as unknown as StoreApi<AppState>;
    const initialSessionIds = store.getState().taskSessionsByTask.itemsByTaskId[TASK_ID] ?? [];
    act(() => {
      store.setState({
        taskSessionsByTask: {
          ...store.getState().taskSessionsByTask,
          itemsByTaskId: { [TASK_ID]: initialSessionIds },
          loadedByTaskId: { [TASK_ID]: true },
        },
      });
    });

    const restoredPanelIds =
      useDockviewStore
        .getState()
        .preMaximizeLayout?.columns[0]?.groups[0]?.panels.map((panel) => panel.id) ?? [];
    expect(restoredPanelIds).toEqual(expect.arrayContaining([activePanelId, siblingPanelId]));
    expect(restoredPanelIds).not.toContain(stalePanelId);
  });
});

const route = {
  operation_id: "workflow-session:task-1:step-b:entry:00000000000000000042:profile::reuse",
  destination_step_id: "step-b",
  entry_identity: "entry:00000000000000000042",
  destination_session_id: ACTIVE_SESSION_ID,
  phase: "committed",
};
describe("useAutoSessionTab focus requests", () => {
  it.each([false, true, "maximized", "hidden"] as const)(
    "delivers and acknowledges workflow focus across restore=%s",
    (restoring) => {
      const { api, activationSequence, centerActivePanelId } =
        makeReorderingAutoSessionApi("files");
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
      if (restoring === true)
        act(() => {
          Object.assign(api, { onDidActivePanelChange: () => ({ dispose: () => {} }) });
          for (const id of [ACTIVE_SESSION_ID, SIBLING_SESSION_ID]) {
            store.getState().setTaskSession({ id, task_id: TASK_ID } as TaskSession);
          }
          store.setState({
            environmentIdBySessionId: {
              [ACTIVE_SESSION_ID]: "env",
              [SIBLING_SESSION_ID]: "env",
            },
          });
          api.getPanel(`session:${SIBLING_SESSION_ID}`)!.api.setActive();
          useDockviewStore.setState({ currentLayoutEnvId: "env" });
          const sync = setupSessionTabSync(api as DockviewApi, store);
          stopTabSync = () => sync.dispose();
          useDockviewStore.setState({ isRestoringLayout: true });
        });
      activationSequence.length = 0;
      if (restoring === "maximized")
        useDockviewStore.setState({ preMaximizeLayout: { columns: [] } });
      if (restoring === "hidden") hiddenSessionIdsFor(api as DockviewApi).add(ACTIVE_SESSION_ID);

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
      if (restoring === true) act(() => useDockviewStore.setState({ isRestoringLayout: false }));

      expect(centerActivePanelId()).toBe(`session:${ACTIVE_SESSION_ID}`);
      expect(acknowledgeSpy).toHaveBeenCalledTimes(1);
      expect(acknowledgeSpy).toHaveBeenCalledWith(1);
      expect(store.getState().tasks.activeSessionId).toBe(ACTIVE_SESSION_ID);
      expect(store.getState().workflowSessionFocus.request).toBeNull();
    },
  );
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.12
it.each([
  [false, false, false],
  [true, false, false],
  [true, true, false],
  [false, false, true],
  [true, false, true],
])(
  "admits explicit route=%s across restore=%s and maximized=%s",
  (explicit, restoring, maximized) => {
    const { api, centerActivePanelId } = makeReorderingAutoSessionApi();
    const environmentId = `route-hidden-${explicit}-${restoring}-${maximized}`;
    const sessions = [ACTIVE_SESSION_ID, SIBLING_SESSION_ID].map(
      (id) => ({ id, task_id: TASK_ID, state: "WAITING_FOR_INPUT" }) as TaskSession,
    );
    const initialState = {
      ...defaultState,
      tasks: { ...defaultState.tasks, activeTaskId: TASK_ID, activeSessionId: ACTIVE_SESSION_ID },
      taskSessions: {
        ...defaultState.taskSessions,
        items: Object.fromEntries(sessions.map((row) => [row.id, row])),
      },
      environmentIdBySessionId: {
        [ACTIVE_SESSION_ID]: environmentId,
        [SIBLING_SESSION_ID]: environmentId,
      },
      taskSessionsByTask: {
        ...defaultState.taskSessionsByTask,
        itemsByTaskId: { [TASK_ID]: sessions },
        loadedByTaskId: { [TASK_ID]: true },
      },
    };
    useDockviewStore.setState({ api, currentLayoutEnvId: environmentId });
    const page = render(
      <StateProvider initialState={initialState}>
        <FocusHarness />
      </StateProvider>,
    );
    const store = capturedStore!;
    if (maximized) {
      act(() => setMaximizedSessions(sessions));
    } else {
      act(() => hideSessionPanel(api, SIBLING_SESSION_ID, TASK_ID));
      expect(api.getPanel(`session:${SIBLING_SESSION_ID}`)).toBeNull();
    }
    if (restoring)
      act(() => {
        Object.assign(api, { onDidActivePanelChange: () => ({ dispose: () => {} }) });
        const sync = setupSessionTabSync(api, store);
        stopTabSync = () => sync.dispose();
        useDockviewStore.setState({ isRestoringLayout: true });
      });
    const incoming = {
      tasks: { ...initialState.tasks, activeSessionId: SIBLING_SESSION_ID },
      taskSessionsByTask: initialState.taskSessionsByTask,
    };
    const owner = captureTaskNavigationSelection(store);
    page.rerender(
      <StateProvider initialState={{}}>
        <StateHydrator
          initialState={incoming}
          requestedSessionId={explicit ? SIBLING_SESSION_ID : undefined}
          taskSelectionAtRequestStart={owner}
        />
        <FocusHarness />
      </StateProvider>,
    );
    if (restoring)
      act(() => {
        store.setState({
          environmentIdBySessionId: {
            [ACTIVE_SESSION_ID]: environmentId,
            [SIBLING_SESSION_ID]: environmentId,
          },
        });
        useDockviewStore.setState({ isRestoringLayout: false });
      });
    expect(hiddenSessionIdsFor(api).has(SIBLING_SESSION_ID)).toBe(!explicit && !maximized);
    expect(centerActivePanelId()).toBe(
      `session:${explicit ? SIBLING_SESSION_ID : ACTIVE_SESSION_ID}`,
    );
    expect(store.getState().tasks.activeSessionId).toBe(
      explicit || maximized ? SIBLING_SESSION_ID : ACTIVE_SESSION_ID,
    );
    if (maximized) expect(useDockviewStore.getState().preMaximizeLayout).not.toBeNull();
    if (explicit && !maximized) {
      act(() => {
        api.getPanel("plan")!.api.setActive();
        store.setState((state) => ({
          taskSessionsByTask: { ...state.taskSessionsByTask, loadedByTaskId: { [TASK_ID]: false } },
        }));
      });
      act(() =>
        store
          .getState()
          .setTaskSessionsForTask(
            TASK_ID,
            sessions,
            captureTaskSessionHydrationEpochs(store.getState(), TASK_ID),
          ),
      );
      expect(centerActivePanelId()).toBe("plan");
    }
  },
);

function setMaximizedSessions(sessions: TaskSession[]) {
  useDockviewStore.setState({
    preMaximizeLayout: {
      columns: [
        {
          id: "center",
          groups: [
            {
              id: "center",
              activePanel: `session:${ACTIVE_SESSION_ID}`,
              panels: sessions.map((row) => ({
                id: `session:${row.id}`,
                component: "chat",
                title: "Agent",
                params: { sessionId: row.id },
              })),
            },
          ],
        },
      ],
    },
  });
}
