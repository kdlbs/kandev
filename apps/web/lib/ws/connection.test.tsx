import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { setWebSocketClient, useWebSocketClient } from "./connection";
import type { WebSocketClient } from "./client";

describe("useWebSocketClient", () => {
  afterEach(() => {
    setWebSocketClient(null);
  });

  it("rerenders mounted consumers when the active client changes", () => {
    const first = {} as WebSocketClient;
    const second = {} as WebSocketClient;
    const { result } = renderHook(() => useWebSocketClient());

    act(() => setWebSocketClient(first));
    expect(result.current).toBe(first);

    act(() => setWebSocketClient(second));
    expect(result.current).toBe(second);
  });
});
