import { describe, expect, it } from "vitest";
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

    dispatch(handlers, "workspace.updated", { id: "ws-1", name: "Platform", unit_id: "unit-new" });

    expect(store.getState().workspaces.items[0].unit_id).toBe("unit-new");
  });

  // An older backend omits the key entirely. Reading a missing key as ""
  // would unplace the workspace, which reads as "reaches nobody".
  it("keeps the current placement when the payload omits unit_id", () => {
    const store = storeWith([workspace()]);
    const handlers = registerWorkspacesHandlers(store);

    dispatch(handlers, "workspace.updated", { id: "ws-1", name: "Platform" });

    expect(store.getState().workspaces.items[0].unit_id).toBe("unit-old");
  });

  it("carries placement onto a workspace created in another tab", () => {
    const store = storeWith([]);
    const handlers = registerWorkspacesHandlers(store);

    dispatch(handlers, "workspace.created", { id: "ws-2", name: "Runtime", unit_id: "unit-new" });

    expect(store.getState().workspaces.items[0].unit_id).toBe("unit-new");
  });
});
