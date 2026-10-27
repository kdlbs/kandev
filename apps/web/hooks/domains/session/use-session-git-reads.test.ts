import { act, cleanup } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WebSocketClient } from "@/lib/ws/client";
import { setWebSocketClient } from "@/lib/ws/connection";
import type { CumulativeDiff, SessionCommit } from "@/lib/state/slices/session-runtime/types";
import { renderSessionRead } from "./session-read-test-helpers";
import { useSessionCommits } from "./use-session-commits";
import { invalidateCumulativeDiffCache, useCumulativeDiff } from "./use-cumulative-diff";
import { registerTasksHandlers } from "@/lib/ws/handlers/tasks";
import { sessionId as toSessionId, taskId as toTaskId, type TaskSession } from "@/lib/types/http";

const COMMITS_ACTION = "session.git.commits";
const DIFF_ACTION = "session.cumulative_diff";
const DELETED_TASK = "deleted-task";
const TASK_DELETED = "task.deleted";

function taskSession(id: string, taskId: string): TaskSession {
  return {
    id: toSessionId(id),
    task_id: toTaskId(taskId),
    state: "WAITING_FOR_INPUT",
    started_at: "2026-10-27T10:00:00Z",
    updated_at: "2026-10-27T10:00:00Z",
  };
}

type Request = { id: string; action: string; payload: { session_id: string } };

class GitReadSocket {
  static instance: GitReadSocket;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  readonly sent: Request[] = [];

  constructor() {
    GitReadSocket.instance = this;
  }

  send(data: string) {
    this.sent.push(JSON.parse(data));
  }

  respond(action: string, payload: unknown, type = "response") {
    const request = this.sent.findLast((entry) => entry.action === action);
    if (!request) throw new Error(`No request for ${action}`);
    this.onmessage?.({ data: JSON.stringify({ type, id: request.id, payload }) });
  }

  respondFor(action: string, sessionId: string, payload: unknown, type = "response") {
    const request = this.sent.findLast(
      (entry) => entry.action === action && entry.payload.session_id === sessionId,
    );
    if (!request) throw new Error(`No request for ${action} and ${sessionId}`);
    this.onmessage?.({ data: JSON.stringify({ type, id: request.id, payload }) });
  }

  close() {}
}

const commit: SessionCommit = {
  id: "commit",
  session_id: "session",
  commit_sha: "head",
  parent_sha: "base",
  author_name: "Test",
  author_email: "test@example.test",
  commit_message: "A committed change",
  committed_at: "2026-10-17T00:00:00Z",
  created_at: "2026-10-17T00:00:00Z",
  files_changed: 1,
  insertions: 1,
  deletions: 0,
};
const diff: CumulativeDiff = {
  session_id: "session",
  base_commit: "base",
  head_commit: "head",
  total_commits: 1,
  files: { "change.txt": { path: "change.txt", status: "added", staged: false, additions: 1 } },
};

let client: WebSocketClient;
beforeEach(() => {
  vi.useFakeTimers();
  vi.stubGlobal("WebSocket", GitReadSocket);
  client = new WebSocketClient("ws://test", undefined, { enabled: false });
  client.connect();
  GitReadSocket.instance.onopen?.();
  setWebSocketClient(client);
});
afterEach(() => {
  cleanup();
  setWebSocketClient(null);
  client.disconnect();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("deleted task Git reads", () => {
  it("does not restart reads when a deleted session is mounted again", async () => {
    const { result, rerender } = renderSessionRead(
      (sessionId: string | null) => useCumulativeDiff(sessionId),
      "session" as string | null,
      (store) => {
        store.setState((state) => ({
          taskSessions: {
            ...state.taskSessions,
            items: { session: taskSession("session", DELETED_TASK) },
          },
        }));
      },
    );
    act(() => {
      const handler = registerTasksHandlers(result.current.store)[TASK_DELETED]!;
      handler({
        type: "notification",
        action: TASK_DELETED,
        payload: { task_id: DELETED_TASK },
      } as Parameters<typeof handler>[0]);
    });
    rerender(null);
    rerender("session");
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(
      GitReadSocket.instance.sent.filter((request) => request.payload.session_id === "session"),
    ).toHaveLength(1);
  });

  it("preserves a sibling commit read when the deleted session owned the shared environment", async () => {
    const { result } = renderSessionRead(
      () => ({ kept: useSessionCommits("other"), deleted: useSessionCommits("session") }),
      undefined,
      (store) => {
        store.setState((state) => ({
          environmentIdBySessionId: { session: "environment", other: "environment" },
          taskSessions: {
            ...state.taskSessions,
            items: { session: taskSession("session", DELETED_TASK) },
          },
        }));
      },
    );
    act(() => {
      const handler = registerTasksHandlers(result.current.store)[TASK_DELETED]!;
      handler({
        type: "notification",
        action: TASK_DELETED,
        payload: { task_id: DELETED_TASK },
      } as Parameters<typeof handler>[0]);
    });
    await act(async () => {
      GitReadSocket.instance.respondFor(COMMITS_ACTION, "other", { commits: [commit] });
      GitReadSocket.instance.respondFor(COMMITS_ACTION, "session", { commits: [] });
    });
    expect(result.current.value.kept.commits).toEqual([commit]);
  });
});

describe("deleted task Git read recovery", () => {
  it("discards a deleted session's late result while a shared-environment session continues", async () => {
    const { result } = renderSessionRead(
      () => ({ deleted: useCumulativeDiff("session"), kept: useCumulativeDiff("other") }),
      undefined,
      (store) => {
        store.setState((state) => ({
          environmentIdBySessionId: { session: "environment", other: "environment" },
          taskSessions: {
            ...state.taskSessions,
            items: {
              session: taskSession("session", DELETED_TASK),
              other: taskSession("other", "kept-task"),
            },
          },
        }));
      },
    );
    act(() => {
      const handler = registerTasksHandlers(result.current.store)[TASK_DELETED]!;
      handler({
        type: "notification",
        action: TASK_DELETED,
        payload: { task_id: DELETED_TASK },
      } as Parameters<typeof handler>[0]);
    });
    await act(async () => {
      GitReadSocket.instance.respondFor(DIFF_ACTION, "session", { cumulative_diff: diff });
      GitReadSocket.instance.respondFor(DIFF_ACTION, "other", { cumulative_diff: diff });
    });
    expect(result.current.value.deleted.diff).toBeNull();
    expect(result.current.value.kept.diff).toEqual(diff);
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(
      GitReadSocket.instance.sent.filter((request) => request.payload.session_id === "session"),
    ).toHaveLength(1);
  });

  it.each(["mapped", "fallback"])(
    "stops retries when a task is deleted with a %s session binding",
    async (binding) => {
      const { result } = renderSessionRead(
        () => useGitReads(),
        undefined,
        (store) => {
          store.setState((state) => ({
            taskSessions: {
              ...state.taskSessions,
              items: { session: taskSession("session", DELETED_TASK) },
            },
            ...(binding === "fallback" ? { environmentIdBySessionId: {} } : {}),
          }));
        },
      );
      await act(async () => rejectReads("INTERNAL_ERROR"));
      act(() => {
        const handler = registerTasksHandlers(result.current.store)[TASK_DELETED]!;
        handler({
          type: "notification",
          action: TASK_DELETED,
          payload: { task_id: DELETED_TASK },
        } as Parameters<typeof handler>[0]);
      });
      await act(async () => vi.advanceTimersByTimeAsync(60_000));
      expect(
        GitReadSocket.instance.sent.filter((request) => request.payload.session_id === "session"),
      ).toHaveLength(2);
    },
  );
});

describe("slow Changes panel Git reads", () => {
  it.each([6500, 29_000])("publishes commits and diff arriving after %i ms", async (delay) => {
    const { result } = renderSessionRead(
      () => ({ commits: useSessionCommits("session"), diff: useCumulativeDiff("session") }),
      undefined,
    );

    await act(async () => vi.advanceTimersByTimeAsync(delay));
    await act(async () => {
      GitReadSocket.instance.respond(COMMITS_ACTION, { success: true, commits: [commit] });
      GitReadSocket.instance.respond(DIFF_ACTION, { cumulative_diff: diff });
    });
    expect(result.current.value.commits.commits).toEqual([commit]);
    expect(result.current.value.diff.diff).toEqual(diff);
    expect(result.current.value.commits.loading).toBe(false);
    expect(result.current.value.diff).toMatchObject({ loading: false, error: null });
    expect(GitReadSocket.instance.sent.map((request) => request.action)).toEqual([
      COMMITS_ACTION,
      DIFF_ACTION,
    ]);
  });

  it("automatically retries timed-out reads once for all consumers and stops after success", async () => {
    const { result } = renderSessionRead(() => [useGitReads(), useGitReads()], undefined);
    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    for (const consumer of result.current.value) {
      expect(consumer.commits.loading).toBe(true);
      expect(consumer.diff).toMatchObject({ loading: true, error: null });
    }
    await act(async () => vi.advanceTimersByTimeAsync(4999));
    expect(GitReadSocket.instance.sent).toHaveLength(2);
    await act(async () => vi.advanceTimersByTimeAsync(1));
    expect(GitReadSocket.instance.sent).toHaveLength(4);
    await act(async () => respondWithData());
    for (const consumer of result.current.value) {
      expect(consumer.commits).toMatchObject({ commits: [commit], loading: false });
      expect(consumer.diff).toMatchObject({ diff, loading: false, error: null });
    }
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(GitReadSocket.instance.sent).toHaveLength(4);
  });

  it.each(["INTERNAL_ERROR", "UNAVAILABLE", "timeout"])(
    "retains the last data through a failed refresh (%s) and accepts an empty recovery",
    async (failure) => {
      const observed: { commits: SessionCommit[]; diff: CumulativeDiff | null }[] = [];
      const { result } = renderSessionRead(() => {
        const value = useGitReads();
        observed.push({ commits: value.commits.commits, diff: value.diff.diff });
        return value;
      }, undefined);
      await act(async () => respondWithData());
      observed.length = 0;
      act(() => {
        result.current.store.getState().bumpSessionCommitsRefetch("session");
        invalidateCumulativeDiffCache(result.current.store, "environment");
      });
      await act(async () => vi.advanceTimersByTimeAsync(200));
      if (failure === "timeout") {
        await act(async () => vi.advanceTimersByTimeAsync(30_000));
      } else {
        await act(async () => rejectReads(failure));
      }
      await act(async () => vi.advanceTimersByTimeAsync(5000));
      expect(GitReadSocket.instance.sent).toHaveLength(6);
      expect(observed.length).toBeGreaterThan(0);
      for (const snapshot of observed) expect(snapshot).toEqual({ commits: [commit], diff });
      await act(async () => {
        GitReadSocket.instance.respond(COMMITS_ACTION, { commits: [] });
        GitReadSocket.instance.respond(DIFF_ACTION, { cumulative_diff: null });
      });
      expect(result.current.value.commits).toMatchObject({ commits: [], loading: false });
      expect(result.current.value.diff).toMatchObject({ diff: null, loading: false, error: null });
    },
  );

  it.each(["FORBIDDEN", "NOT_FOUND"])("does not retry a permanent %s error", async (code) => {
    const { result } = renderSessionRead(() => useGitReads(), undefined);
    await act(async () => rejectReads(code));
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(GitReadSocket.instance.sent).toHaveLength(2);
    expect(result.current.value.commits.loading).toBe(false);
    expect(result.current.value.diff).toMatchObject({ loading: false, error: code });
  });

  it.each(["unmount", "session switch", "disconnect"])(
    "cancels scheduled recovery on %s",
    async (change) => {
      const { result, unmount, rerender } = renderSessionRead(useGitReads, "session" as string);
      await act(async () => rejectReads("UNAVAILABLE"));
      expect(result.current.value.diff.loading).toBe(true);
      if (change === "unmount") unmount();
      if (change === "session switch") rerender("other");
      if (change === "disconnect") {
        act(() => result.current.store.getState().setConnectionStatus("disconnected"));
      }
      await act(async () => vi.advanceTimersByTimeAsync(60_000));
      expect(
        GitReadSocket.instance.sent.filter((request) => request.payload.session_id === "session"),
      ).toHaveLength(2);
    },
  );
});

function useGitReads(sessionId = "session") {
  return { commits: useSessionCommits(sessionId), diff: useCumulativeDiff(sessionId) };
}

function respondWithData() {
  GitReadSocket.instance.respond(COMMITS_ACTION, { commits: [commit] });
  GitReadSocket.instance.respond(DIFF_ACTION, { cumulative_diff: diff });
}

function rejectReads(code: string) {
  GitReadSocket.instance.respond(COMMITS_ACTION, { code, message: code }, "error");
  GitReadSocket.instance.respond(DIFF_ACTION, { code, message: code }, "error");
}
