import { beforeEach, describe, expect, it, vi } from "vitest";
import type { WebSocketClient, SessionGitRefreshResponse } from "@/lib/ws/client";
import { createAppStore } from "@/lib/state/store";
import { requestGitStatusRefresh, retainGitRefreshScope } from "./git-status-refresh-coordinator";

const SESSION = "session-a";

type Deferred<T> = {
  promise: Promise<T>;
  resolve: (value: T) => void;
};

function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function statusResponse(
  path: string,
  sessionId = SESSION,
  environmentId = sessionId,
): SessionGitRefreshResponse {
  return {
    success: true,
    session_id: sessionId,
    task_environment_id: environmentId,
    mode: "fresh",
    status_state: "ready",
    snapshots: [
      {
        type: "notification",
        action: "session.git.event",
        payload: {
          type: "status_update",
          session_id: sessionId,
          task_environment_id: environmentId,
          timestamp: "2026-09-30T10:00:00.000Z",
          status: {
            status_state: "ready",
            files_complete: true,
            detail_state: "ready",
            branch: "main",
            remote_branch: null,
            modified: [path],
            added: [],
            deleted: [],
            untracked: [],
            renamed: [],
            ahead: 0,
            behind: 0,
            remote_ahead: 0,
            remote_behind: 0,
            files: { [path]: { path, status: "modified", staged: false } },
          },
        },
      },
    ],
  };
}

function refreshClient(requests: Deferred<SessionGitRefreshResponse>[]) {
  const statusListeners: Array<(status: "connected") => void> = [];
  return {
    getStatus: () => "connected" as const,
    onConnectionStatus: (listener: (status: "connected") => void) => {
      statusListeners.push(listener);
      return () => undefined;
    },
    refreshSessionData: vi.fn(() => {
      const request = deferred<SessionGitRefreshResponse>();
      requests.push(request);
      return request.promise;
    }),
  } as unknown as WebSocketClient;
}

describe("Git status refresh coordinator", () => {
  beforeEach(() => {
    vi.useRealTimers();
  });

  it("starts a live replacement after StrictMode-style release and retain", async () => {
    const store = createAppStore();
    const requests: Deferred<SessionGitRefreshResponse>[] = [];
    const client = refreshClient(requests);

    const releaseFirst = retainGitRefreshScope(client, SESSION);
    const firstAttempt = requestGitStatusRefresh(client, store, SESSION, SESSION);
    expect(requests).toHaveLength(1);

    releaseFirst();
    const releaseReplacement = retainGitRefreshScope(client, SESSION);
    const replacementAttempt = requestGitStatusRefresh(client, store, SESSION, SESSION);
    expect(requests).toHaveLength(2);

    requests[0].resolve(statusResponse("stale.txt"));
    await firstAttempt;
    expect(store.getState().gitStatus.byEnvironmentId[SESSION]).toBeUndefined();
    expect(store.getState().gitStatus.refreshByEnvironmentId?.[SESSION]?.state).toBe("pending");

    requests[1].resolve(statusResponse("current.txt"));
    await replacementAttempt;
    expect(store.getState().gitStatus.byEnvironmentId[SESSION]?.files).toHaveProperty(
      "current.txt",
    );
    expect(store.getState().gitStatus.byEnvironmentId[SESSION]?.files).not.toHaveProperty(
      "stale.txt",
    );
    expect(store.getState().gitStatus.refreshByEnvironmentId?.[SESSION]).toBeUndefined();
    releaseReplacement();
  });

  it("keeps shared work alive while another session owns the same environment", async () => {
    const store = createAppStore();
    const requests: Deferred<SessionGitRefreshResponse>[] = [];
    const client = refreshClient(requests);
    const siblingSession = "session-b";
    const environmentId = "environment-shared";
    store.getState().registerSessionEnvironment(SESSION, environmentId);
    store.getState().registerSessionEnvironment(siblingSession, environmentId);
    const releaseFirst = retainGitRefreshScope(client, environmentId);
    const releaseSibling = retainGitRefreshScope(client, environmentId);
    const attempt = requestGitStatusRefresh(client, store, SESSION, environmentId);

    releaseFirst();
    expect(requests[0].promise).toBeDefined();
    const siblingAttempt = requestGitStatusRefresh(client, store, siblingSession, environmentId);
    expect(requests).toHaveLength(1);
    requests[0].resolve(statusResponse("shared.txt", SESSION, environmentId));
    await Promise.all([attempt, siblingAttempt]);

    expect(store.getState().gitStatus.byEnvironmentId[environmentId]?.files).toHaveProperty(
      "shared.txt",
    );
    releaseSibling();
  });
});
