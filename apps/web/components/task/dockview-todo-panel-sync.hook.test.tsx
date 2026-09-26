import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render } from "@testing-library/react";
import type { StoreApi } from "zustand";
import type { DockviewApi } from "dockview-react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { AppState } from "@/lib/state/store";
import type { SessionId, TaskId } from "@/lib/types/http";
import { useSyncTodoPanel } from "./dockview-todo-panel-sync";

const TASK_ID = "task-1" as TaskId;
const SESSION_ID = "session-1" as SessionId;
const WORKSPACE_ID = "workspace-1";

let storeApi: StoreApi<AppState>;
function Harness() {
  storeApi = useAppStoreApi();
  useSyncTodoPanel();
  return null;
}

const addPanel = vi.fn();
const close = vi.fn();
const pendingFrames = new Map<number, FrameRequestCallback>();
let nextFrameId = 0;

function setDockviewState(panel: { id: string; api: { close: typeof close } } | undefined): void {
  useDockviewStore.setState({
    api: {
      getPanel: (id: string) => (panel && id === "todos" ? panel : undefined),
      groups: [{ id: "group-center" }],
      addPanel,
    } as unknown as DockviewApi,
    centerGroupId: "group-center",
    isRestoringLayout: false,
    userDefaultLayout: null,
    preMaximizeLayout: null,
  });
}

function renderHook(overrides: Partial<AppState> = {}): void {
  render(
    <StateProvider initialState={{ ...defaultState, ...overrides }}>
      <Harness />
    </StateProvider>,
  );
}

/** Runs exactly one animation frame: the outer rAF fires and schedules the
 *  inner rAF, which stays pending until the next call, mirroring the
 *  browser's two-frame scheduling. */
function flushOneFrame(): void {
  act(() => {
    const due = [...pendingFrames.entries()];
    for (const [id, callback] of due) {
      pendingFrames.delete(id);
      callback(0);
    }
  });
}

function flushAllFrames(): void {
  while (pendingFrames.size > 0) {
    flushOneFrame();
  }
}

function sessionedStateOverrides(showTodoListPanel: boolean, loaded: boolean): Partial<AppState> {
  return {
    userSettings: {
      ...defaultState.userSettings,
      showTodoListPanel,
      showTodoListPanelOnlyWhenNotEmpty: true,
      loaded,
    },
    tasks: { ...defaultState.tasks, activeTaskId: TASK_ID, activeSessionId: SESSION_ID },
    workspaces: { ...defaultState.workspaces, activeId: WORKSPACE_ID },
  };
}

beforeEach(() => {
  pendingFrames.clear();
  nextFrameId = 0;
  addPanel.mockReset();
  close.mockReset();
  vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
    const id = ++nextFrameId;
    pendingFrames.set(id, callback);
    return id;
  });
  vi.spyOn(window, "cancelAnimationFrame").mockImplementation((id) => {
    pendingFrames.delete(id);
  });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  useDockviewStore.setState({ api: null });
});

describe("useSyncTodoPanel", () => {
  it("adds the panel once live todos arrive when the sub-option is on", () => {
    setDockviewState(undefined);
    renderHook(sessionedStateOverrides(true, true));

    // Empty todo list suppresses the add.
    flushAllFrames();
    expect(addPanel).not.toHaveBeenCalled();

    // A live sessionTodos update re-runs the sync and pins the panel.
    act(() => {
      storeApi.setState({
        sessionTodos: {
          bySessionId: { [SESSION_ID]: [{ description: "Do it", status: "in_progress" }] },
        },
      });
    });
    flushAllFrames();
    expect(addPanel).toHaveBeenCalledWith(expect.objectContaining({ id: "todos" }));
  });

  it("removes a materialized panel for a sessionless task when the preference is off", () => {
    setDockviewState({ id: "todos", api: { close } });
    renderHook({
      ...sessionedStateOverrides(false, true),
      tasks: { ...defaultState.tasks, activeTaskId: TASK_ID, activeSessionId: null },
    });

    flushAllFrames();
    expect(close).toHaveBeenCalledOnce();
  });

  it("never auto-adds for a sessionless task even when the preference is on", () => {
    setDockviewState(undefined);
    renderHook({
      ...sessionedStateOverrides(true, true),
      tasks: { ...defaultState.tasks, activeTaskId: TASK_ID, activeSessionId: null },
    });

    flushAllFrames();
    expect(addPanel).not.toHaveBeenCalled();
  });

  it("cancels a pending sync when the preference changes between the frames", () => {
    setDockviewState(undefined);
    renderHook(sessionedStateOverrides(true, true));
    flushOneFrame(); // outer fired, inner pending

    // The preference flips before the inner frame: the effect cleanup must
    // cancel the pending inner, and the new effect's sync must see the
    // updated preference (off -> nothing to add or remove).
    act(() => {
      storeApi.setState({
        userSettings: { ...storeApi.getState().userSettings, showTodoListPanel: false },
      });
    });
    flushAllFrames();

    expect(addPanel).not.toHaveBeenCalled();
    expect(close).not.toHaveBeenCalled();
  });

  it("does not apply a stale task's sync after a task switch mid-flight", () => {
    setDockviewState(undefined);
    renderHook(sessionedStateOverrides(true, true));

    // Todos arrive for task-1 and re-schedule the sync, but the user
    // switches to a different (sessionless) task before any frame fires.
    act(() => {
      storeApi.setState({
        sessionTodos: {
          bySessionId: { [SESSION_ID]: [{ description: "Do it", status: "in_progress" }] },
        },
      });
    });
    act(() => {
      storeApi.setState({
        tasks: { ...defaultState.tasks, activeTaskId: "task-2" as TaskId, activeSessionId: null },
      });
    });

    flushAllFrames();
    // Neither the cancelled task-1 frames (identity guard) nor the new
    // sessionless task may add a panel.
    expect(addPanel).not.toHaveBeenCalled();
  });

  it("does nothing before settings hydrate, then syncs once loaded", () => {
    setDockviewState(undefined);
    renderHook({
      ...sessionedStateOverrides(true, false),
      // Sub-option off so the add is not gated on the (empty) todo list.
      userSettings: {
        ...defaultState.userSettings,
        showTodoListPanel: true,
        showTodoListPanelOnlyWhenNotEmpty: false,
        loaded: false,
      },
    });

    flushAllFrames();
    expect(addPanel).not.toHaveBeenCalled();

    act(() => {
      storeApi.setState({
        userSettings: { ...storeApi.getState().userSettings, loaded: true },
      });
    });
    flushAllFrames();
    expect(addPanel).toHaveBeenCalledWith(expect.objectContaining({ id: "todos" }));
  });

  it("removes the panel when the preference turns off", () => {
    setDockviewState({ id: "todos", api: { close } });
    renderHook(sessionedStateOverrides(true, true));

    flushAllFrames();
    expect(close).not.toHaveBeenCalled();

    act(() => {
      storeApi.setState({
        userSettings: { ...storeApi.getState().userSettings, showTodoListPanel: false },
      });
    });
    flushAllFrames();
    expect(close).toHaveBeenCalledOnce();
  });
});
