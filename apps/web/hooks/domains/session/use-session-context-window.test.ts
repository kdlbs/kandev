import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  unsubscribe: vi.fn(),
  subscribeSession: vi.fn(),
  setContextWindow: vi.fn(),
  state: {
    contextWindow: { bySessionId: {} },
    taskSessions: { items: {} },
    connection: { status: "connected" },
    setContextWindow: vi.fn(),
  },
}));

mocks.subscribeSession.mockReturnValue(mocks.unsubscribe);

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ subscribeSession: mocks.subscribeSession }),
}));

import { useSessionContextWindow } from "./use-session-context-window";

afterEach(() => {
  cleanup();
  mocks.unsubscribe.mockClear();
  mocks.subscribeSession.mockClear();
});

describe("useSessionContextWindow detail demand", () => {
  it("does not subscribe while its chat panel is hidden", () => {
    renderHook(() => useSessionContextWindow("session-1", false));

    expect(mocks.subscribeSession).not.toHaveBeenCalled();
  });

  it("subscribes when revealed and releases the session on hide", () => {
    const { rerender, unmount } = renderHook(
      ({ detailActive }: { detailActive: boolean }) =>
        useSessionContextWindow("session-1", detailActive),
      { initialProps: { detailActive: false } },
    );

    expect(mocks.subscribeSession).not.toHaveBeenCalled();

    rerender({ detailActive: true });
    expect(mocks.subscribeSession).toHaveBeenCalledTimes(1);
    expect(mocks.subscribeSession).toHaveBeenCalledWith("session-1");

    rerender({ detailActive: false });
    expect(mocks.unsubscribe).toHaveBeenCalledTimes(1);
    unmount();
    expect(mocks.unsubscribe).toHaveBeenCalledTimes(1);
  });
});
