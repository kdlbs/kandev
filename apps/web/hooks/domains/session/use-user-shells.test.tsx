import { act, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { setWebSocketClient } from "@/lib/ws/connection";
import type { WebSocketClient } from "@/lib/ws/client";
import { useUserShells } from "./use-user-shells";
import { useSessionCommits } from "./use-session-commits";
import { useCumulativeDiff } from "./use-cumulative-diff";
import { deferred, renderSessionRead } from "./session-read-test-helpers";

afterEach(() => {
  cleanup();
  setWebSocketClient(null);
});

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.2
describe("shared session read initialization", () => {
  it.each(["shells", "commits", "diff"] as const)(
    "shares %s across consumers and survives the initiating consumer leaving",
    async (kind) => {
      const response = deferred<unknown>();
      const request = vi.fn(() => response.promise);
      setWebSocketClient({ request } as unknown as WebSocketClient);
      const useRead = {
        shells: useUserShells,
        commits: useSessionCommits,
        diff: useCumulativeDiff,
      }[kind];
      const { result, rerender } = renderSessionRead(
        ({ first }: { first: string | null }) => [useRead(first), useRead("session")],
        { first: "session" as string | null },
      );
      expect(request).toHaveBeenCalledTimes(1);
      rerender({ first: null });
      await act(async () => {
        response.resolve({ shells: [{ id: "shell" }], commits: [], cumulative_diff: null });
        await response.promise;
      });
      const remaining = result.current.value[1];
      expect("isLoading" in remaining ? remaining.isLoading : remaining.loading).toBe(false);
      rerender({ first: "session" });
      expect(request).toHaveBeenCalledTimes(1);
    },
  );
});

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
describe("shell response ownership", () => {
  it.each([false, true])(
    "settles a failed list while preserving cached shells: %s",
    async (cached) => {
      const response = deferred<unknown>();
      const request = vi.fn(() => response.promise);
      setWebSocketClient({ request } as unknown as WebSocketClient);
      const shells = cached ? [{ terminalId: "cached", running: false, closable: true }] : [];
      const { result } = renderSessionRead(
        () => useUserShells("environment", "task"),
        undefined,
        (store) =>
          store.setState({
            userShells: {
              ...store.getState().userShells,
              byEnvironmentId: { environment: shells },
            },
          }),
      );
      expect(result.current.value.isLoading).toBe(true);
      await act(async () => response.reject(new Error("temporary shell-list failure")));
      expect(result.current.value.isLoaded).toBe(true);
      expect(result.current.value.isLoading).toBe(false);
      expect(result.current.value.shells).toEqual(shells);
      expect(request).toHaveBeenCalledTimes(1);
    },
  );

  it("does not settle a replacement workspace when the old shell list fails", async () => {
    const stale = deferred<unknown>();
    const fresh = deferred<unknown>();
    const request = vi.fn().mockReturnValueOnce(stale.promise).mockReturnValueOnce(fresh.promise);
    setWebSocketClient({ request } as unknown as WebSocketClient);
    const { result } = renderSessionRead(() => useUserShells("environment", "task"), undefined);
    act(() => result.current.store.setState({ workspaceContextGeneration: 1 }));
    await act(async () => stale.reject(new Error("old request failed")));
    expect(result.current.value.isLoaded).toBe(false);
    expect(result.current.value.isLoading).toBe(true);
    await act(async () => fresh.resolve({ shells: [{ id: "current" }] }));
    expect(result.current.value.shells.map((shell) => shell.terminalId)).toEqual(["current"]);
  });

  it("does not let an older environment-only list overwrite task-scoped shells", async () => {
    const legacy = deferred<{ shells: { id: string }[] }>();
    const scoped = deferred<{ shells: { id: string }[] }>();
    const request = vi.fn().mockReturnValueOnce(legacy.promise).mockReturnValueOnce(scoped.promise);
    setWebSocketClient({ request } as unknown as WebSocketClient);
    const { result, rerender } = renderSessionRead(
      (taskId: string | null) => useUserShells("environment", taskId),
      null as string | null,
    );
    rerender("task");
    expect(request).toHaveBeenLastCalledWith(
      "user_shell.list",
      { task_environment_id: "environment", task_id: "task", include_parked: true },
      10000,
    );
    await act(async () => scoped.resolve({ shells: [{ id: "current" }] }));
    await act(async () => legacy.resolve({ shells: [{ id: "obsolete" }] }));
    expect(result.current.value.shells.map((shell) => shell.terminalId)).toEqual(["current"]);
  });

  it("rejects a response from a retired workspace even when the environment is reused", async () => {
    const stale = deferred<{ shells: { id: string }[] }>();
    const fresh = deferred<{ shells: { id: string }[] }>();
    const request = vi.fn().mockReturnValueOnce(stale.promise).mockReturnValueOnce(fresh.promise);
    setWebSocketClient({ request } as unknown as WebSocketClient);
    const { result } = renderSessionRead(() => useUserShells("environment", "task"), undefined);
    act(() => result.current.store.setState({ workspaceContextGeneration: 1 }));
    await act(async () => fresh.resolve({ shells: [{ id: "current" }] }));
    await act(async () => stale.resolve({ shells: [{ id: "obsolete" }] }));
    expect(result.current.value.shells.map((shell) => shell.terminalId)).toEqual(["current"]);
  });
});
