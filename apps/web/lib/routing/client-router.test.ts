import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useParams, usePathname, useRouter, useSearchParams } from "./client-router";

function setLocation(path: string) {
  window.history.replaceState({}, "", path);
}

describe("client router adapter", () => {
  it("pushes and replaces browser history routes", () => {
    setLocation("/");
    const scrollTo = vi.fn();
    vi.stubGlobal("scrollTo", scrollTo);
    const { result } = renderHook(() => useRouter());

    act(() => result.current.push("/tasks"));
    expect(window.location.pathname).toBe("/tasks");
    expect(scrollTo).toHaveBeenCalledWith(0, 0);

    act(() => result.current.replace("/stats?range=7d", { scroll: false }));
    expect(window.location.pathname).toBe("/stats");
    expect(window.location.search).toBe("?range=7d");
    expect(scrollTo).toHaveBeenCalledTimes(1);
    vi.unstubAllGlobals();
  });

  it("returns current path and search params", () => {
    setLocation("/stats?range=7d");

    expect(renderHook(() => usePathname()).result.current).toBe("/stats");
    expect(renderHook(() => useSearchParams()).result.current.get("range")).toBe("7d");
  });

  it("derives known route params from the current path", () => {
    setLocation("/t/task-123");

    expect(renderHook(() => useParams()).result.current).toEqual({ taskId: "task-123" });
  });

  it("refreshes by reloading the document", () => {
    const reload = vi.fn();
    vi.stubGlobal("location", { ...window.location, reload });
    const { result } = renderHook(() => useRouter());

    act(() => result.current.refresh());

    expect(reload).toHaveBeenCalledOnce();
    vi.unstubAllGlobals();
  });
});
