import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, renderHook } from "@testing-library/react";
import { QueryClient } from "@tanstack/react-query";

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => null,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: () => null,
  useAppStoreApi: () => null,
}));

import { useVisibilityBackfill } from "./use-session-messages";
import { qk } from "@/lib/query/keys";

function setVisibility(value: "visible" | "hidden") {
  Object.defineProperty(document, "visibilityState", { configurable: true, value });
  document.dispatchEvent(new Event("visibilitychange"));
}

describe("useVisibilityBackfill", () => {
  let queryClient: QueryClient;
  let invalidateSpy: ReturnType<typeof vi.fn<unknown[], unknown>>;

  beforeEach(() => {
    vi.clearAllMocks();
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    invalidateSpy = vi.fn<unknown[], unknown>(() => Promise.resolve());
    queryClient.invalidateQueries =
      invalidateSpy as unknown as typeof queryClient.invalidateQueries;
  });

  afterEach(() => {
    cleanup();
  });

  it("invalidates the messages query when the tab becomes visible", () => {
    renderHook(() => useVisibilityBackfill("sess-1", queryClient));
    setVisibility("visible");
    expect(invalidateSpy).toHaveBeenCalledTimes(1);
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: qk.session.messages("sess-1") });
  });

  it("does not invalidate when the tab becomes hidden", () => {
    renderHook(() => useVisibilityBackfill("sess-1", queryClient));
    setVisibility("hidden");
    expect(invalidateSpy).not.toHaveBeenCalled();
  });

  it("does nothing when sessionId is null", () => {
    renderHook(() => useVisibilityBackfill(null, queryClient));
    setVisibility("visible");
    expect(invalidateSpy).not.toHaveBeenCalled();
  });

  it("removes the listener on unmount", () => {
    const { unmount } = renderHook(() => useVisibilityBackfill("sess-1", queryClient));
    unmount();
    setVisibility("visible");
    expect(invalidateSpy).not.toHaveBeenCalled();
  });

  it("re-registers when sessionId changes", () => {
    const { rerender } = renderHook(
      ({ id }: { id: string | null }) => useVisibilityBackfill(id, queryClient),
      { initialProps: { id: "sess-1" } },
    );
    setVisibility("visible");
    expect(invalidateSpy).toHaveBeenLastCalledWith({ queryKey: qk.session.messages("sess-1") });

    rerender({ id: "sess-2" });
    setVisibility("visible");
    expect(invalidateSpy).toHaveBeenLastCalledWith({ queryKey: qk.session.messages("sess-2") });
  });
});
