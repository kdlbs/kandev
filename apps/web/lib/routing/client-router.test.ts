import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { clearNavigationBlockerForTests, setNavigationBlocker } from "./navigation-guard";
import { useParams, usePathname, useRouter, useSearchParams } from "./client-router";

const NAV_POSITION_KEY = "__kandevNavigationPosition";

function setLocation(path: string) {
  window.history.replaceState({}, "", path);
}

/** Simulates a history popstate at the given location for router tests. */
function popStateAt(position: number) {
  window.dispatchEvent(new PopStateEvent("popstate", { state: { [NAV_POSITION_KEY]: position } }));
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

  it("derives the Kubernetes executor settings route parameter", () => {
    setLocation("/settings/executors/k8s/executor-123");

    expect(renderHook(() => useParams()).result.current).toMatchObject({
      executorId: "executor-123",
    });
  });

  it("refreshes by reloading the document", () => {
    const reload = vi.fn();
    vi.stubGlobal("location", { ...window.location, reload });
    const { result } = renderHook(() => useRouter());

    act(() => result.current.refresh());

    expect(reload).toHaveBeenCalledOnce();
  });

  describe("navigation guard", () => {
    function setupGuardedRouter() {
      setLocation("/settings/general/appearance");
      const intents: Array<{ proceed: () => void; cancel: () => void }> = [];
      setNavigationBlocker((intent) => intents.push(intent));
      const go = vi.spyOn(window.history, "go").mockImplementation(() => undefined);
      const back = vi.spyOn(window.history, "back").mockImplementation(() => popStateAt(0));
      const { result } = renderHook(() => useRouter());
      return { result, intents, go, back };
    }

    it("guards imperative push and history navigation", () => {
      const { result, intents, go, back } = setupGuardedRouter();

      act(() => result.current.push("/tasks"));
      expect(window.location.pathname).toBe("/settings/general/appearance");
      act(() => intents.shift()?.proceed());
      expect(window.location.pathname).toBe("/tasks");

      act(() => result.current.back());
      expect(back).toHaveBeenCalledOnce();
      expect(intents).toHaveLength(1);
      act(() => intents.shift()?.proceed());
      window.dispatchEvent(new PopStateEvent("popstate", { state: window.history.state }));
      window.dispatchEvent(new PopStateEvent("popstate", { state: null }));
      expect(intents).toHaveLength(0);
      expect(go).toHaveBeenCalledTimes(2);
    });

    it("consumes a cancelled blocked pop's restoration popstate when no newer navigation superseded it", () => {
      const { result, intents, go } = setupGuardedRouter();

      act(() => result.current.push("/tasks"));
      act(() => intents.pop()?.proceed()); // allow the push -> currentPosition 1
      act(() => result.current.back()); // blocked pop to position 0; restoration to 1 pending
      expect(intents).toHaveLength(1);

      act(() => {
        intents[0].cancel(); // cancel without any newer navigation
      });

      // The in-flight restoration popstate for the blocked entry is consumed
      // without touching currentPosition (still 1).
      act(() => popStateAt(1));

      // A later real back to position 0 is a fresh guarded pop (delta -1),
      // not a silent acceptance.
      act(() => popStateAt(0));
      expect(intents).toHaveLength(2);
      expect(go).toHaveBeenCalled();
    });

    it("forgets a cancelled blocked pop once a newer navigation supersedes it", () => {
      const { result, intents, go } = setupGuardedRouter();

      act(() => result.current.push("/tasks"));
      act(() => intents.pop()?.proceed()); // allow the push -> currentPosition 1
      act(() => result.current.back()); // blocked pop to position 0
      expect(intents).toHaveLength(1);

      // Duplicate-like flow: cancel, then push a new route. The push
      // supersedes the pending traversal, so the browser may never emit its
      // popstate; the guard must forget the cancelled restoration instead of
      // arming a stale position that could eat a later navigation.
      act(() => {
        intents[0].cancel();
        result.current.push("/settings/agents/mock-agent/profiles/copy-1");
        intents.pop()?.proceed(); // allow the duplicate push -> currentPosition 2
      });

      // A popstate to the blocked entry's position arrives late (or is a
      // real back): it must be treated as a fresh guarded pop (delta -1 from
      // position 2), not consumed as stale.
      act(() => popStateAt(1));
      expect(intents).toHaveLength(2);
      expect(go).toHaveBeenCalled();
    });
  });
});
