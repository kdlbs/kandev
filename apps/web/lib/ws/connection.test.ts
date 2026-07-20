import { beforeEach, describe, expect, it, vi } from "vitest";
import type { WebSocketClient } from "./client";
import { getWebSocketClient, setWebSocketClient, subscribeWebSocketClient } from "./connection";

function fakeClient(id: string): WebSocketClient {
  return { id } as unknown as WebSocketClient;
}

describe("WebSocket connection registry", () => {
  beforeEach(() => {
    setWebSocketClient(null);
  });

  it("notifies subscribers immediately and when the active client changes", () => {
    const first = fakeClient("first");
    const second = fakeClient("second");
    const listener = vi.fn();

    setWebSocketClient(first);
    const unsubscribe = subscribeWebSocketClient(listener);
    setWebSocketClient(second);
    setWebSocketClient(null);

    expect(listener).toHaveBeenNthCalledWith(1, first);
    expect(listener).toHaveBeenNthCalledWith(2, second);
    expect(listener).toHaveBeenNthCalledWith(3, null);
    expect(getWebSocketClient()).toBeNull();

    unsubscribe();
  });

  it("stops notifying after unsubscribe", () => {
    const listener = vi.fn();
    const unsubscribe = subscribeWebSocketClient(listener);

    unsubscribe();
    setWebSocketClient(fakeClient("next"));

    expect(listener).toHaveBeenCalledTimes(1);
    expect(listener).toHaveBeenCalledWith(null);
  });
});
