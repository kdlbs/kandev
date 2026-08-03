import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ConnectionStatus } from "@/lib/types/connection";
import { createAppStore } from "@/lib/state/store";
import { useWebSocket } from "./use-websocket";

const mocks = vi.hoisted(() => ({
  onStatusChange: null as ((status: ConnectionStatus) => void) | null,
}));

vi.mock("@/lib/ws/client", () => ({
  WebSocketClient: class {
    constructor(_url: string, onStatusChange: (status: ConnectionStatus) => void) {
      mocks.onStatusChange = onStatusChange;
    }

    connect = vi.fn();
    disconnect = vi.fn();
    subscribeUser = vi.fn();
    on = vi.fn(() => vi.fn());
  },
}));

vi.mock("@/lib/ws/router", () => ({
  registerWsHandlers: vi.fn(() => ({ handlers: {}, dispose: vi.fn() })),
}));
vi.mock("@/lib/ws/connection", () => ({ setWebSocketClient: vi.fn() }));
vi.mock("@/lib/debug/log", () => ({ createDebugLogger: () => vi.fn() }));

describe("useWebSocket", () => {
  afterEach(() => {
    mocks.onStatusChange = null;
    vi.useRealTimers();
  });

  it("reflects a sustained WebSocket outage in the UI store and clears on reconnect", () => {
    vi.useFakeTimers();
    const store = createAppStore();
    const { unmount } = renderHook(() => useWebSocket(store, "ws://kandev.test/ws"));

    act(() => {
      mocks.onStatusChange?.("reconnecting");
      vi.advanceTimersByTime(3_000);
    });

    expect(store.getState().connection.issueSeverity).toBe("unstable");

    act(() => mocks.onStatusChange?.("connected"));

    expect(store.getState().connection.issueSeverity).toBe("none");
    unmount();
  });

  it("clears an active outage when replacing the WebSocket URL", () => {
    vi.useFakeTimers();
    const store = createAppStore();
    const { rerender, unmount } = renderHook(
      ({ url }: { url: string }) => useWebSocket(store, url),
      { initialProps: { url: "ws://kandev.test/first" } },
    );

    act(() => {
      mocks.onStatusChange?.("reconnecting");
      vi.advanceTimersByTime(3_000);
    });
    expect(store.getState().connection.issueSeverity).toBe("unstable");

    rerender({ url: "ws://kandev.test/replacement" });

    expect(store.getState().connection.issueSeverity).toBe("none");
    unmount();
  });
});
