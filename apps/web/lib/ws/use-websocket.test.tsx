import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WebSocketClient } from "@/lib/ws/client";
import { registerWsHandlers } from "@/lib/ws/router";
import { getWebSocketClient, setWebSocketClient, subscribeWebSocketClient } from "./connection";
import { useWebSocket } from "./use-websocket";

vi.mock("@/lib/ws/router", () => ({
  registerWsHandlers: vi.fn(),
}));

class FakeSocket {
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onerror: (() => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  send = vi.fn();
  close = vi.fn(() => {
    this.onclose?.({} as CloseEvent);
  });

  constructor(readonly url: string) {}
}

function makeStore() {
  return {
    getState: () => ({
      setConnectionStatus: vi.fn(),
    }),
  } as unknown as StoreApi<AppState>;
}

function dispatchTaskUpdated(client: WebSocketClient) {
  (
    client as unknown as {
      handleParsedMessage: (message: unknown) => void;
    }
  ).handleParsedMessage({
    type: "notification",
    action: "task.updated",
    payload: { task_id: "task-1" },
  });
}

describe("useWebSocket", () => {
  beforeEach(() => {
    vi.stubGlobal("WebSocket", FakeSocket);
    setWebSocketClient(null);
  });

  afterEach(() => {
    setWebSocketClient(null);
    vi.unstubAllGlobals();
  });

  it("publishes the client only after legacy handlers are registered", () => {
    const calls: string[] = [];
    vi.mocked(registerWsHandlers).mockReturnValue({
      "task.updated": () => calls.push("legacy"),
    } as never);
    const unsubscribe = subscribeWebSocketClient((client) => {
      if (!client) return;
      client.on("task.updated", () => calls.push("bridge"));
    });

    const { unmount } = renderHook(() => useWebSocket(makeStore(), "ws://example.test/ws"));
    const client = getWebSocketClient();
    expect(client).not.toBeNull();

    dispatchTaskUpdated(client as WebSocketClient);

    expect(calls).toEqual(["legacy", "bridge"]);
    unsubscribe();
    unmount();
  });
});
