import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, renderHook } from "@testing-library/react";

const mockRequest = vi.fn();
const mockSetMessages = vi.fn();
const SESSION_ID = "sess-1";
const MESSAGE_LIST = "message.list";

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mockRequest }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: () => null,
  useAppStoreApi: () => ({
    getState: () => ({
      messages: { bySession: {} },
      setMessages: mockSetMessages,
    }),
  }),
}));

import { useVisibilityBackfill } from "./use-session-messages";

function setVisibility(value: "visible" | "hidden") {
  Object.defineProperty(document, "visibilityState", { configurable: true, value });
  document.dispatchEvent(new Event("visibilitychange"));
}

describe("useVisibilityBackfill", () => {
  let store: { getState: () => unknown };

  beforeEach(() => {
    vi.clearAllMocks();
    mockRequest.mockResolvedValue({ messages: [], has_more: false });
    store = {
      getState: () => ({ messages: { bySession: {} }, setMessages: mockSetMessages }),
    };
  });

  afterEach(() => {
    cleanup();
  });

  it("fetches when the tab becomes visible", () => {
    renderHook(() => useVisibilityBackfill(SESSION_ID, store as never));
    setVisibility("visible");
    expect(mockRequest).toHaveBeenCalledTimes(1);
    expect(mockRequest).toHaveBeenCalledWith(
      MESSAGE_LIST,
      expect.objectContaining({ session_id: SESSION_ID }),
      expect.any(Number),
    );
  });

  it("fetches when the Kandev window regains focus", () => {
    renderHook(() => useVisibilityBackfill(SESSION_ID, store as never));

    window.dispatchEvent(new Event("focus"));

    expect(mockRequest).toHaveBeenCalledWith(
      MESSAGE_LIST,
      expect.objectContaining({ session_id: SESSION_ID }),
      expect.any(Number),
    );
  });

  it("does not fetch when the tab becomes hidden", () => {
    renderHook(() => useVisibilityBackfill(SESSION_ID, store as never));
    setVisibility("hidden");
    expect(mockRequest).not.toHaveBeenCalled();
  });

  it("does nothing when sessionId is null", () => {
    renderHook(() => useVisibilityBackfill(null, store as never));
    setVisibility("visible");
    expect(mockRequest).not.toHaveBeenCalled();
  });

  it("removes the listener on unmount", () => {
    const { unmount } = renderHook(() => useVisibilityBackfill(SESSION_ID, store as never));
    unmount();
    setVisibility("visible");
    expect(mockRequest).not.toHaveBeenCalled();
  });

  it("re-registers when sessionId changes", () => {
    const { rerender } = renderHook(
      ({ id }: { id: string | null }) => useVisibilityBackfill(id, store as never),
      { initialProps: { id: SESSION_ID } },
    );
    setVisibility("visible");
    expect(mockRequest).toHaveBeenLastCalledWith(
      MESSAGE_LIST,
      expect.objectContaining({ session_id: SESSION_ID }),
      expect.any(Number),
    );

    rerender({ id: "sess-2" });
    setVisibility("visible");
    expect(mockRequest).toHaveBeenLastCalledWith(
      MESSAGE_LIST,
      expect.objectContaining({ session_id: "sess-2" }),
      expect.any(Number),
    );
  });
});
