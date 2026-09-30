import { describe, expect, it, vi } from "vitest";
import { createStore } from "zustand";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerWorkspacesHandlers } from "@/lib/ws/handlers/workspaces";
import type { WsHandlers } from "@/lib/ws/handlers/types";

// The handler map is partial by type; a missing entry is a real failure here,
// not something to silently skip.
function dispatch(handlers: WsHandlers, type: string, payload: unknown) {
  const handler = handlers[type as keyof WsHandlers];
  if (!handler) throw new Error(`no handler registered for ${type}`);
  (handler as (message: unknown) => void)({ type, payload });
}

type WorkspaceItem = AppState["workspaces"]["items"][number];
const SECOND_WORKSPACE_ID = "workspace-2";
const BASE_UPDATED_PAYLOAD = { id: "ws-1", name: "Platform" } as const;
const WORKSPACE_UPDATED = "workspace.updated";
const WORKSPACE_CREATED = "workspace.created";
const WORKSPACE_DELETED = "workspace.deleted";

function storeWith(items: WorkspaceItem[]): StoreApi<AppState> {
  return createStore<AppState>(
    () =>
      ({
        workspaces: { items, activeId: items[0]?.id ?? null },
      }) as unknown as AppState,
  );
}

function workspace(overrides: Partial<WorkspaceItem> = {}): WorkspaceItem {
  return {
    id: "ws-1",
    name: "Platform",
    description: null,
    owner_id: "user-1",
    default_executor_id: null,
    default_environment_id: null,
    default_agent_profile_id: null,
    default_config_agent_profile_id: null,
    unit_id: "unit-old",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  } as WorkspaceItem;
}

describe("workspace.updated placement", () => {
  // Placement is reach: a move made by another admin, or in another tab, is
  // what grants and withdraws access. If the merge drops unit_id the board
  // keeps rendering against the unit it just left.
  it("applies a new unit_id", () => {
    const store = storeWith([workspace()]);
    const handlers = registerWorkspacesHandlers(store);

    dispatch(handlers, WORKSPACE_UPDATED, { ...BASE_UPDATED_PAYLOAD, unit_id: "unit-new" });

    expect(store.getState().workspaces.items[0].unit_id).toBe("unit-new");
  });

  // An older backend omits the key entirely. Reading a missing key as ""
  // would unplace the workspace, which reads as "reaches nobody".
  it("keeps the current placement when the payload omits unit_id", () => {
    const store = storeWith([workspace()]);
    const handlers = registerWorkspacesHandlers(store);

    dispatch(handlers, WORKSPACE_UPDATED, { ...BASE_UPDATED_PAYLOAD });

    expect(store.getState().workspaces.items[0].unit_id).toBe("unit-old");
  });

  it("carries placement onto a workspace created in another tab", () => {
    const store = storeWith([]);
    const handlers = registerWorkspacesHandlers(store);

    dispatch(handlers, WORKSPACE_CREATED, { id: "ws-2", name: "Runtime", unit_id: "unit-new" });

    expect(store.getState().workspaces.items[0].unit_id).toBe("unit-new");
  });

  it("bumps the active workspace revision when the first workspace becomes active", () => {
    const store = storeWith([]);
    const handlers = registerWorkspacesHandlers(store);

    dispatch(handlers, WORKSPACE_CREATED, { id: "ws-2", name: "Runtime" });

    expect(store.getState().workspaces.activeId).toBe("ws-2");
    expect(store.getState().workspaces.activeIdRevision).toBe(1);
  });
});

describe("workspace.updated ACP idle-suspension policy", () => {
  // The policy round-trips between tabs: a save in one tab must land in the
  // other tab's store, or the settings form there re-saves stale values.
  it("applies updated policy fields", () => {
    const store = storeWith([workspace()]);
    const handlers = registerWorkspacesHandlers(store);

    dispatch(handlers, WORKSPACE_UPDATED, {
      id: "ws-1",
      name: "Platform",
      acp_idle_suspension_enabled: true,
      acp_idle_timeout_minutes: 30,
    });

    expect(store.getState().workspaces.items[0].acp_idle_suspension_enabled).toBe(true);
    expect(store.getState().workspaces.items[0].acp_idle_timeout_minutes).toBe(30);
  });

  // An older backend omits both keys entirely. Reading a missing key as its
  // default would flip suspension off (or reset the timeout) on a tab that
  // already knows the saved policy.
  it("keeps the current values when the payload omits both keys", () => {
    const store = storeWith([
      workspace({ acp_idle_suspension_enabled: true, acp_idle_timeout_minutes: 45 }),
    ]);
    const handlers = registerWorkspacesHandlers(store);

    dispatch(handlers, WORKSPACE_UPDATED, { ...BASE_UPDATED_PAYLOAD });

    expect(store.getState().workspaces.items[0].acp_idle_suspension_enabled).toBe(true);
    expect(store.getState().workspaces.items[0].acp_idle_timeout_minutes).toBe(45);
  });

  it("keeps each field independently when only one key is present", () => {
    const store = storeWith([
      workspace({ acp_idle_suspension_enabled: true, acp_idle_timeout_minutes: 45 }),
    ]);
    const handlers = registerWorkspacesHandlers(store);

    dispatch(handlers, WORKSPACE_UPDATED, {
      id: "ws-1",
      name: "Platform",
      acp_idle_timeout_minutes: 60,
    });

    expect(store.getState().workspaces.items[0].acp_idle_suspension_enabled).toBe(true);
    expect(store.getState().workspaces.items[0].acp_idle_timeout_minutes).toBe(60);
  });

  it("carries the policy onto a workspace created in another tab", () => {
    const store = storeWith([]);
    const handlers = registerWorkspacesHandlers(store);

    dispatch(handlers, WORKSPACE_CREATED, {
      id: "ws-2",
      name: "Runtime",
      acp_idle_suspension_enabled: true,
      acp_idle_timeout_minutes: 15,
    });

    expect(store.getState().workspaces.items[0].acp_idle_suspension_enabled).toBe(true);
    expect(store.getState().workspaces.items[0].acp_idle_timeout_minutes).toBe(15);
  });
});

describe("workspace.deleted queue cleanup", () => {
  const WORKSPACE_ID = "workspace-1";

  it("clears normalized task sessions without relying on queue metadata", () => {
    const clearQueueStatus = vi.fn();
    let state = {
      workspaces: {
        items: [{ id: WORKSPACE_ID }, { id: SECOND_WORKSPACE_ID }],
        activeId: WORKSPACE_ID,
      },
      workflows: { items: [], activeId: null },
      kanban: {
        workflowId: "workflow-1",
        steps: [],
        tasks: [{ id: "task-1", workspaceId: WORKSPACE_ID }],
      },
      kanbanMulti: { snapshots: {} },
      taskSessions: {
        items: {
          "session-1": {
            id: "session-1",
            task_id: "task-1",
            queue_incarnation_id: "incarnation-1",
          },
        },
      },
      clearQueueStatus,
    } as unknown as AppState;
    const store = {
      getState: () => state,
      setState: (updater: AppState | Partial<AppState> | ((value: AppState) => AppState)) => {
        state = (
          typeof updater === "function" ? updater(state) : { ...state, ...updater }
        ) as AppState;
      },
    } as StoreApi<AppState>;

    registerWorkspacesHandlers(store)[WORKSPACE_DELETED]!({
      payload: { id: WORKSPACE_ID },
    } as never);

    expect(clearQueueStatus).toHaveBeenCalledOnce();
    expect(clearQueueStatus).toHaveBeenCalledWith("session-1");
  });

  it("bumps the active workspace revision when deletion selects a fallback", () => {
    const clearQueueStatus = vi.fn();
    const first = { ...workspace(), id: WORKSPACE_ID };
    const second = { ...workspace(), id: SECOND_WORKSPACE_ID };
    const store = createStore<AppState>(
      () =>
        ({
          workspaces: {
            items: [first, second],
            activeId: WORKSPACE_ID,
            activeIdRevision: 4,
          },
          workflows: { items: [], activeId: null },
          kanban: { workflowId: null, steps: [], tasks: [] },
          kanbanMulti: { snapshots: {} },
          taskSessions: { items: {} },
          clearQueueStatus,
        }) as unknown as AppState,
    );

    registerWorkspacesHandlers(store)[WORKSPACE_DELETED]!({
      payload: { id: WORKSPACE_ID },
    } as never);

    expect(store.getState().workspaces.activeId).toBe(SECOND_WORKSPACE_ID);
    expect(store.getState().workspaces.activeIdRevision).toBe(5);
  });

  it("preserves the active workspace revision when deletion leaves the selection unchanged", () => {
    const clearQueueStatus = vi.fn();
    const first = { ...workspace(), id: WORKSPACE_ID };
    const second = { ...workspace(), id: SECOND_WORKSPACE_ID };
    const store = createStore<AppState>(
      () =>
        ({
          workspaces: {
            items: [first, second],
            activeId: SECOND_WORKSPACE_ID,
            activeIdRevision: 4,
          },
          workflows: { items: [], activeId: null },
          kanban: { workflowId: null, steps: [], tasks: [] },
          kanbanMulti: { snapshots: {} },
          taskSessions: { items: {} },
          clearQueueStatus,
        }) as unknown as AppState,
    );

    registerWorkspacesHandlers(store)[WORKSPACE_DELETED]!({
      payload: { id: WORKSPACE_ID },
    } as never);

    expect(store.getState().workspaces.activeId).toBe(SECOND_WORKSPACE_ID);
    expect(store.getState().workspaces.activeIdRevision).toBe(4);
  });
});
