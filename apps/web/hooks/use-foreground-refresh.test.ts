import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useForegroundRefresh } from "./use-foreground-refresh";

function setVisibility(value: DocumentVisibilityState) {
  Object.defineProperty(document, "visibilityState", { configurable: true, value });
}

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((innerResolve) => {
    resolve = innerResolve;
  });
  return { promise, resolve };
}

describe("useForegroundRefresh", () => {
  beforeEach(() => {
    setVisibility("visible");
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("refreshes when the Kandev window regains focus", () => {
    const refresh = vi.fn();
    renderHook(() => useForegroundRefresh(refresh));

    act(() => window.dispatchEvent(new Event("focus")));

    expect(refresh).toHaveBeenCalledOnce();
  });

  it("does not refresh while the document is hidden", () => {
    const refresh = vi.fn();
    renderHook(() => useForegroundRefresh(refresh));
    setVisibility("hidden");

    act(() => window.dispatchEvent(new Event("focus")));

    expect(refresh).not.toHaveBeenCalled();
  });

  it("coalesces a foreground event burst while the first request is in flight", () => {
    const request = deferred();
    const refresh = vi.fn(() => request.promise);
    renderHook(() => useForegroundRefresh(refresh));

    act(() => {
      window.dispatchEvent(new Event("focus"));
      document.dispatchEvent(new Event("visibilitychange"));
      window.dispatchEvent(new Event("pageshow"));
      window.dispatchEvent(new Event("online"));
    });

    expect(refresh).toHaveBeenCalledOnce();
    act(() => request.resolve());
  });
});
