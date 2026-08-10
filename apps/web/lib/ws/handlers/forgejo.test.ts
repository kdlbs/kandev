import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerForgejoHandlers } from "./forgejo";

const config = {
  workspace_id: "ws-a",
  origin: "https://forgejo.example",
  username: "alice",
  has_secret: true,
  has_webhook_secret: false,
  last_ok: true,
  created_at: "",
  updated_at: "",
};

function makeStore(activeId: string | null) {
  const setForgejoConfigState = vi.fn();
  const markForgejoTaskLinksUpdated = vi.fn();
  const markForgejoWorkspaceDataUpdated = vi.fn();
  return {
    setForgejoConfigState,
    markForgejoTaskLinksUpdated,
    markForgejoWorkspaceDataUpdated,
    store: {
      getState: () => ({
        workspaces: { activeId },
        setForgejoConfigState,
        markForgejoTaskLinksUpdated,
        markForgejoWorkspaceDataUpdated,
      }),
    } as unknown as StoreApi<AppState>,
  };
}

describe("Forgejo WebSocket handlers", () => {
  it("updates the active workspace connection", () => {
    const { store, setForgejoConfigState } = makeStore("ws-a");
    registerForgejoHandlers(store)["forgejo.config.updated"]!({
      type: "notification",
      action: "forgejo.config.updated",
      payload: config,
    });
    expect(setForgejoConfigState).toHaveBeenCalledWith(
      "ws-a",
      expect.objectContaining({ origin: "https://forgejo.example" }),
    );
  });

  it("does not leak a connection update across workspaces", () => {
    const { store, setForgejoConfigState } = makeStore("ws-b");
    registerForgejoHandlers(store)["forgejo.config.updated"]!({
      type: "notification",
      action: "forgejo.config.updated",
      payload: config,
    });
    expect(setForgejoConfigState).not.toHaveBeenCalled();
  });

  it("marks active task links stale after a scoped update", () => {
    const { store, markForgejoTaskLinksUpdated } = makeStore("ws-a");
    registerForgejoHandlers(store)["forgejo.task_links.updated"]!({
      type: "notification",
      action: "forgejo.task_links.updated",
      payload: { workspace_id: "ws-a", task_id: "task-7" },
    });
    expect(markForgejoTaskLinksUpdated).toHaveBeenCalledWith("ws-a", "task-7");
  });

  it("does not leak task link updates across workspaces", () => {
    const { store, markForgejoTaskLinksUpdated } = makeStore("ws-b");
    registerForgejoHandlers(store)["forgejo.task_links.updated"]!({
      type: "notification",
      action: "forgejo.task_links.updated",
      payload: { workspace_id: "ws-a", task_id: "task-7" },
    });
    expect(markForgejoTaskLinksUpdated).not.toHaveBeenCalled();
  });

  it("marks active workspace watch data stale", () => {
    const { store, markForgejoWorkspaceDataUpdated } = makeStore("ws-a");
    registerForgejoHandlers(store)["forgejo.workspace_data.updated"]!({
      type: "notification",
      action: "forgejo.workspace_data.updated",
      payload: { workspace_id: "ws-a" },
    });
    expect(markForgejoWorkspaceDataUpdated).toHaveBeenCalledWith("ws-a");
  });
});
