import { describe, expect, it } from "vitest";
import { createStore } from "zustand";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerWorkspacesHandlers } from "@/lib/ws/handlers/workspaces";
import type { WsHandlers } from "@/lib/ws/handlers/types";

type WorkspaceItem = AppState["workspaces"]["items"][number];

function dispatch(handlers: WsHandlers, type: string, payload: unknown) {
  const handler = handlers[type as keyof WsHandlers];
  if (!handler) throw new Error(`no handler registered for ${type}`);
  (handler as (message: unknown) => void)({ type, payload });
}

function storeWith(items: WorkspaceItem[]): StoreApi<AppState> {
  return createStore<AppState>(
    () => ({ workspaces: { items, activeId: items[0]?.id ?? null } }) as unknown as AppState,
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
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  } as WorkspaceItem;
}

describe("workspace events: unanswered-question inbox settings", () => {
  it("applies updated settings from another tab", () => {
    const store = storeWith([workspace()]);
    dispatch(registerWorkspacesHandlers(store), "workspace.updated", {
      id: "ws-1",
      name: "Platform",
      clarification_unanswered_inbox_enabled: true,
      clarification_unanswered_retention_days: 30,
    });
    const item = store.getState().workspaces.items[0];
    expect(item.clarification_unanswered_inbox_enabled).toBe(true);
    expect(item.clarification_unanswered_retention_days).toBe(30);
  });

  it("keeps the current settings when an older backend omits the keys", () => {
    const store = storeWith([
      workspace({
        clarification_unanswered_inbox_enabled: true,
        clarification_unanswered_retention_days: 7,
      }),
    ]);
    dispatch(registerWorkspacesHandlers(store), "workspace.updated", {
      id: "ws-1",
      name: "Platform",
    });
    const item = store.getState().workspaces.items[0];
    expect(item.clarification_unanswered_inbox_enabled).toBe(true);
    expect(item.clarification_unanswered_retention_days).toBe(7);
  });

  it("carries settings onto a workspace created in another tab", () => {
    const store = storeWith([]);
    dispatch(registerWorkspacesHandlers(store), "workspace.created", {
      id: "ws-2",
      name: "New",
      clarification_unanswered_inbox_enabled: false,
      clarification_unanswered_retention_days: 14,
    });
    const item = store.getState().workspaces.items[0];
    expect(item.clarification_unanswered_inbox_enabled).toBe(false);
    expect(item.clarification_unanswered_retention_days).toBe(14);
  });
});
