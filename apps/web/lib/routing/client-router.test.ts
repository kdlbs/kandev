import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { clearNavigationBlockerForTests, setNavigationBlocker } from "./navigation-guard";
import { useParams, usePathname, useRouter, useSearchParams } from "./client-router";

function setLocation(path: string) {
  window.history.replaceState({}, "", path);
}

afterEach(() => {
  clearNavigationBlockerForTests();
  vi.unstubAllGlobals();
});

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

  it("derives nested settings agent profile params", () => {
    setLocation("/settings/agents/mock-agent/profiles/profile-123");

    expect(renderHook(() => useParams()).result.current).toMatchObject({
      agentId: "mock-agent",
      profileId: "profile-123",
    });
  });

  it("refreshes by reloading the document", () => {
    const reload = vi.fn();
    vi.stubGlobal("location", { ...window.location, reload });
    const { result } = renderHook(() => useRouter());

    act(() => result.current.refresh());

    expect(reload).toHaveBeenCalledOnce();
  });

  it("guards imperative push and history navigation", () => {
    setLocation("/settings/general/appearance");
    const attempts: Array<() => void> = [];
    setNavigationBlocker((intent) => attempts.push(intent.proceed));
    const go = vi.spyOn(window.history, "go").mockImplementation(() => undefined);
    const back = vi.spyOn(window.history, "back").mockImplementation(() => {
      window.dispatchEvent(
        new PopStateEvent("popstate", {
          state: { __kandevNavigationPosition: 0 },
        }),
      );
    });
    const { result } = renderHook(() => useRouter());

    act(() => result.current.push("/tasks"));
    expect(window.location.pathname).toBe("/settings/general/appearance");

    act(() => attempts.shift()?.());
    expect(window.location.pathname).toBe("/tasks");

    act(() => result.current.back());
    expect(back).toHaveBeenCalledOnce();
    expect(attempts).toHaveLength(1);
    act(() => attempts.shift()?.());
    window.dispatchEvent(new PopStateEvent("popstate", { state: window.history.state }));
    expect(attempts).toHaveLength(0);
    expect(go).toHaveBeenCalledTimes(2);
  });
});
