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
  return {
    setForgejoConfigState,
    store: {
      getState: () => ({ workspaces: { activeId }, setForgejoConfigState }),
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
});
