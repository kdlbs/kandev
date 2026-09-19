import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import { registerSessionHostnamesHandlers } from "./session-hostnames";

function makeStore() {
  return createAppStore();
}

function hostnameMessage(hostname: string): BackendMessageMap["auth.session.hostname.resolved"] {
  return {
    type: "notification",
    action: "auth.session.hostname.resolved",
    payload: {
      ip: "192.0.2.10",
      hostname,
      resolved_at: "2026-08-18T12:00:00.000000001Z",
    },
  };
}

describe("session hostname websocket handler", () => {
  // Reviewer-requested contract coverage for the identity epoch fence.
  it("rejects an old identity handler and accepts the replacement handler", () => {
    const store = makeStore();
    const oldHandler = registerSessionHostnamesHandlers(store)["auth.session.hostname.resolved"];

    store.getState().setAuthState({
      mode: "enabled",
      authenticated: true,
      user: {
        id: "user-2",
        email: "user@example.test",
        display_name: "User",
        role: "member",
        status: "active",
      },
    });
    oldHandler?.(hostnameMessage("old-user.example.test"));
    expect(store.getState().sessionHostnames).toEqual({});

    const currentHandler =
      registerSessionHostnamesHandlers(store)["auth.session.hostname.resolved"];
    currentHandler?.(hostnameMessage("current-user.example.test"));
    expect(store.getState().sessionHostnames["192.0.2.10"]).toEqual({
      hostname: "current-user.example.test",
      resolvedAt: "2026-08-18T12:00:00.000000001Z",
    });
  });
});
