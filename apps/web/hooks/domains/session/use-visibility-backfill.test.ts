import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { makeQueryClient } from "@/lib/query/client";

const mockListTaskSessionMessages = vi.fn();
const mockMergeMessages = vi.fn();

vi.mock("@/lib/api/domains/session-api", () => ({
  fetchTaskSession: vi.fn(),
  listSessionTurns: vi.fn(),
  listTaskSessionMessages: (...args: unknown[]) => mockListTaskSessionMessages(...args),
  listTaskSessions: vi.fn(),
  searchSessionMessages: vi.fn(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: () => null,
  useAppStoreApi: () => ({
    getState: () => ({
      messages: { bySession: {} },
      mergeMessages: mockMergeMessages,
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
    mockListTaskSessionMessages.mockResolvedValue({ messages: [], has_more: false });
    store = {
      getState: () => ({ messages: { bySession: {} }, mergeMessages: mockMergeMessages }),
    };
  });

  afterEach(() => {
    cleanup();
  });

  it("fetches when the tab becomes visible", async () => {
    const queryClient = makeQueryClient();
    renderHook(() => useVisibilityBackfill("sess-1", store as never, queryClient));
    setVisibility("visible");
    await waitFor(() => expect(mockListTaskSessionMessages).toHaveBeenCalledTimes(1));
    expect(mockListTaskSessionMessages).toHaveBeenCalledWith(
      "sess-1",
      { limit: 100, sort: "desc" },
      expect.any(Object),
    );
  });

  it("does not fetch when the tab becomes hidden", () => {
    renderHook(() => useVisibilityBackfill("sess-1", store as never, makeQueryClient()));
    setVisibility("hidden");
    expect(mockListTaskSessionMessages).not.toHaveBeenCalled();
  });

  it("does nothing when sessionId is null", () => {
    renderHook(() => useVisibilityBackfill(null, store as never, makeQueryClient()));
    setVisibility("visible");
    expect(mockListTaskSessionMessages).not.toHaveBeenCalled();
  });

  it("removes the listener on unmount", () => {
    const { unmount } = renderHook(() =>
      useVisibilityBackfill("sess-1", store as never, makeQueryClient()),
    );
    unmount();
    setVisibility("visible");
    expect(mockListTaskSessionMessages).not.toHaveBeenCalled();
  });

  it("re-registers when sessionId changes", async () => {
    const queryClient = makeQueryClient();
    const { rerender } = renderHook(
      ({ id }: { id: string | null }) => useVisibilityBackfill(id, store as never, queryClient),
      { initialProps: { id: "sess-1" } },
    );
    setVisibility("visible");
    await waitFor(() =>
      expect(mockListTaskSessionMessages).toHaveBeenLastCalledWith(
        "sess-1",
        { limit: 100, sort: "desc" },
        expect.any(Object),
      ),
    );

    rerender({ id: "sess-2" });
    setVisibility("visible");
    await waitFor(() =>
      expect(mockListTaskSessionMessages).toHaveBeenLastCalledWith(
        "sess-2",
        { limit: 100, sort: "desc" },
        expect.any(Object),
      ),
    );
  });
});
