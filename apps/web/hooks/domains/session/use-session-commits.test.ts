import { act, cleanup } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setWebSocketClient } from "@/lib/ws/connection";
import type { WebSocketClient } from "@/lib/ws/client";
import type { SessionCommit } from "@/lib/state/slices/session-runtime/types";
import { useSessionCommits } from "./use-session-commits";
import { deferred, renderSessionRead } from "./session-read-test-helpers";

const request = vi.fn();
function commit(sha: string, insertions = 1): SessionCommit {
  return {
    id: sha,
    session_id: "session",
    commit_sha: sha,
    parent_sha: "parent",
    author_name: "Test",
    author_email: "test@example.test",
    commit_message: sha,
    committed_at: "2026-09-28T12:00:00Z",
    created_at: "2026-09-28T12:00:00Z",
    files_changed: 1,
    insertions,
    deletions: 0,
  };
}

beforeEach(() => {
  vi.useFakeTimers();
  request.mockReset();
  setWebSocketClient({ request } as unknown as WebSocketClient);
});
afterEach(() => {
  cleanup();
  setWebSocketClient(null);
  vi.useRealTimers();
});
const flush = () =>
  act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.2
// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
describe("authoritative shared commit snapshots", () => {
  it("rejects a late snapshot after the session moves to another environment", async () => {
    const old = deferred<{ commits: SessionCommit[] }>();
    const current = deferred<{ commits: SessionCommit[] }>();
    request.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise);
    const { result } = renderSessionRead(() => useSessionCommits("session"), undefined);
    act(() => result.current.store.setState({ environmentIdBySessionId: { session: "new-env" } }));
    expect(request).toHaveBeenCalledTimes(2);
    await act(async () => current.resolve({ commits: [commit("current")] }));
    await act(async () => old.resolve({ commits: [commit("obsolete")] }));
    expect(result.current.value.commits.map((entry) => entry.commit_sha)).toEqual(["current"]);
  });

  it("replaces prefilled event statistics with an authoritative snapshot", async () => {
    request.mockResolvedValue({ commits: [commit("a", 50)] });
    const { result } = renderSessionRead(
      () => useSessionCommits("session"),
      undefined,
      (store) => store.getState().setSessionCommits("session", [commit("a", 0)]),
    );
    await flush();
    expect(result.current.value.commits).toEqual([commit("a", 50)]);
    expect(result.current.value.loading).toBe(false);
  });

  it("preserves a newer live commit against an initial empty snapshot", async () => {
    const response = deferred<{ commits: SessionCommit[] }>();
    request.mockReturnValue(response.promise);
    const { result } = renderSessionRead(() => useSessionCommits("session"), undefined);
    act(() => result.current.store.getState().addSessionCommit("session", commit("live")));
    await act(async () => response.resolve({ commits: [] }));
    expect(result.current.value.commits).toEqual([commit("live")]);
    expect(result.current.value.loading).toBe(false);
  });

  it("retries not-ready reads once for all consumers while retaining loading and existing data", async () => {
    request
      .mockResolvedValueOnce({ commits: [], ready: false })
      .mockResolvedValue({ commits: [commit("ready")] });
    const { result } = renderSessionRead(
      () => [useSessionCommits("session"), useSessionCommits("session")],
      undefined,
    );
    await flush();
    expect(request).toHaveBeenCalledTimes(1);
    expect(result.current.value.map((v) => v.loading)).toEqual([true, true]);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(request).toHaveBeenCalledTimes(2);
    expect(result.current.value.map((v) => v.commits)).toEqual([
      [commit("ready")],
      [commit("ready")],
    ]);
    expect(result.current.value.map((v) => v.loading)).toEqual([false, false]);
  });

  it.each([true, false])("does not retry a ready or terminal session: ready=%s", async (ready) => {
    request.mockResolvedValue(
      ready
        ? { commits: [commit("kept")] }
        : { ready: false, reason: "session_terminal", commits: [] },
    );
    const { result } = renderSessionRead(
      () => useSessionCommits("session"),
      undefined,
      (store) => store.getState().setSessionCommits("session", [commit("kept")]),
    );
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000);
    });
    expect(request).toHaveBeenCalledTimes(1);
    expect(result.current.value.commits).toEqual([commit("kept")]);
    expect(result.current.value.loading).toBe(false);
  });
});

describe("commit read subscription lifecycle", () => {
  it("cancels a not-ready retry when the last consumer leaves", async () => {
    request.mockResolvedValue({ ready: false });
    const { result, unmount } = renderSessionRead(() => useSessionCommits("session"), undefined);
    const store = result.current.store;
    await flush();
    unmount();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000);
    });
    expect(request).toHaveBeenCalledTimes(1);
    expect(store.getState().sessionCommits.loading.environment).toBe(false);
  });

  it("does not fetch without a session or connection", async () => {
    request.mockResolvedValue({ commits: [] });
    const { result, rerender } = renderSessionRead(
      (id: string | null) => useSessionCommits(id),
      null as string | null,
    );
    expect(request).not.toHaveBeenCalled();
    act(() => result.current.store.getState().setConnectionStatus("disconnected"));
    rerender("session");
    expect(request).not.toHaveBeenCalled();
    act(() => result.current.store.getState().setConnectionStatus("connected"));
    await flush();
    expect(request).toHaveBeenCalledTimes(1);
  });

  it("refetches after teardown clears the stored snapshot", async () => {
    request.mockResolvedValue({ commits: [commit("a")] });
    const { result, rerender } = renderSessionRead(
      (id: string | null) => useSessionCommits(id),
      "session" as string | null,
    );
    await flush();
    rerender(null);
    act(() => result.current.store.getState().clearSessionCommits("session"));
    rerender("session");
    await flush();
    expect(request).toHaveBeenCalledTimes(2);
    expect(result.current.value.commits).toEqual([commit("a")]);
  });
});

describe("commit invalidation ownership", () => {
  it("keeps existing commits during refresh and accepts an authoritative empty result", async () => {
    const response = deferred<{ commits: SessionCommit[] }>();
    request
      .mockResolvedValueOnce({ commits: [commit("old")] })
      .mockReturnValueOnce(response.promise);
    const { result } = renderSessionRead(() => useSessionCommits("session"), undefined);
    await flush();
    act(() => result.current.store.getState().bumpSessionCommitsRefetch("session"));
    expect(result.current.value.commits).toEqual([commit("old")]);
    await act(async () => response.resolve({ commits: [] }));
    expect(result.current.value.commits).toEqual([]);
  });

  it("ignores an invalidated result and drains one follow-up for multiple trigger bumps", async () => {
    const first = deferred<{ commits: SessionCommit[] }>();
    const second = deferred<{ commits: SessionCommit[] }>();
    request.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const { result } = renderSessionRead(
      () => [useSessionCommits("session"), useSessionCommits("session")],
      undefined,
    );
    act(() => {
      result.current.store.getState().bumpSessionCommitsRefetch("session");
      result.current.store.getState().bumpSessionCommitsRefetch("session");
    });
    expect(request).toHaveBeenCalledTimes(1);
    await act(async () => first.resolve({ commits: [commit("obsolete")] }));
    expect(request).toHaveBeenCalledTimes(2);
    expect(result.current.value[0].commits).toEqual([]);
    expect(result.current.value[0].loading).toBe(true);
    await act(async () => second.resolve({ commits: [commit("fresh")] }));
    expect(result.current.value.map((v) => v.commits)).toEqual([
      [commit("fresh")],
      [commit("fresh")],
    ]);
  });

  it("preserves authoritative-empty permission through a not-ready retry", async () => {
    request.mockResolvedValueOnce({ ready: false, commits: [] }).mockResolvedValue({ commits: [] });
    const { result } = renderSessionRead(
      () => useSessionCommits("session"),
      undefined,
      (store) => {
        store.getState().setSessionCommits("session", [commit("old")]);
        store.getState().bumpSessionCommitsRefetch("session");
      },
    );
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(result.current.value.commits).toEqual([]);
  });
});

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
describe("commits environment round trips", () => {
  it.each([false, true])(
    "retires the original binding across A to B to A (batched=%s)",
    async (batched) => {
      const old = deferred<unknown>();
      const intermediate = deferred<unknown>();
      const fresh = deferred<unknown>();
      request.mockReturnValueOnce(old.promise);
      if (!batched) request.mockReturnValueOnce(intermediate.promise);
      request.mockReturnValue(fresh.promise);
      const { result } = renderSessionRead(() => useSessionCommits("session"), undefined);
      const move = (environment: string) =>
        result.current.store.setState({ environmentIdBySessionId: { session: environment } });
      if (batched) {
        act(() => {
          move("new-env");
          move("environment");
        });
      } else {
        act(() => move("new-env"));
        act(() => move("environment"));
      }
      await act(async () => old.resolve({ commits: [commit("obsolete")] }));
      expect(result.current.value.commits).toEqual([]);
      expect(request).toHaveBeenCalledTimes(batched ? 2 : 3);
      await act(async () => fresh.resolve({ commits: [commit("fresh")] }));
      await act(async () => intermediate.resolve({ commits: [commit("obsolete")] }));
      expect(result.current.value.commits).toEqual([commit("fresh")]);
      expect(result.current.value.loading).toBe(false);
    },
  );
});
