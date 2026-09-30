import { beforeEach, describe, expect, it, vi } from "vitest";
import type { WebSocketClient, SessionGitRefreshResponse } from "@/lib/ws/client";
import type { GitStatusUpdateEvent } from "@/lib/types/git-events";
import { createAppStore } from "@/lib/state/store";
import { requestGitStatusRefresh, retainGitRefreshScope } from "./git-status-refresh-coordinator";

const SESSION = "session-a";
const TRACKER_ID = "tracker-a";
const CURRENT_FILE = "current.txt";
const EARLIER_TIMESTAMP = "2026-09-30T10:00:01.000Z";
const CURRENT_TIMESTAMP = "2026-09-30T10:00:02.000Z";

type Deferred<T> = {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (reason?: unknown) => void;
};

function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
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

function orderedResponse(
  revision: number,
  timestamp: string,
  state: "ready" | "unavailable" | "loading" = "ready",
): SessionGitRefreshResponse {
  const response = statusResponse("stale.txt");
  const event = response.snapshots[0].payload as GitStatusUpdateEvent;
  event.timestamp = timestamp;
  event.status.status_state = state;
  event.status.files_complete = state === "ready";
  event.status.detail_state = state === "ready" ? "ready" : "unavailable";
  event.status.error_code = state === "ready" ? undefined : "status_unavailable";
  event.status.tracker_id = TRACKER_ID;
  event.status.tracker_epoch = 1;
  event.status.snapshot_revision = revision;
  return response;
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

describe("Git status refresh coordinator scope ownership", () => {
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

    requests[1].resolve(statusResponse(CURRENT_FILE));
    await replacementAttempt;
    expect(store.getState().gitStatus.byEnvironmentId[SESSION]?.files).toHaveProperty(CURRENT_FILE);
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

describe("Git status refresh coordinator snapshot ordering", () => {
  beforeEach(() => {
    vi.useRealTimers();
  });

  it("keeps newer unavailable quality when correlated responses carry only older snapshots", async () => {
    const store = createAppStore();
    const requests: Deferred<SessionGitRefreshResponse>[] = [];
    const client = refreshClient(requests);
    store.getState().setGitStatusRefresh(SESSION, "", {
      state: "unavailable",
      error_code: "details_unavailable",
      tracker_id: TRACKER_ID,
      tracker_epoch: 1,
      snapshot_revision: 7,
      timestamp: CURRENT_TIMESTAMP,
    });
    const release = retainGitRefreshScope(client, SESSION);
    const attempt = requestGitStatusRefresh(client, store, SESSION, SESSION);

    requests[0].resolve(orderedResponse(6, EARLIER_TIMESTAMP));
    await vi.waitFor(() => expect(requests).toHaveLength(2));
    requests[1].resolve(orderedResponse(6, EARLIER_TIMESTAMP));
    await attempt;

    expect(store.getState().gitStatus.byEnvironmentId[SESSION]).toBeUndefined();
    expect(store.getState().gitStatus.refreshByEnvironmentRepo?.[SESSION]?.[""]).toMatchObject({
      state: "unavailable",
      error_code: "details_unavailable",
      tracker_id: TRACKER_ID,
      snapshot_revision: 7,
    });
    release();
  });
});

describe("Git status refresh coordinator replay cancellation", () => {
  beforeEach(() => {
    vi.useRealTimers();
  });

  it("marks pending details unavailable when replay is cancelled with a nullable branch", async () => {
    vi.useFakeTimers();
    const store = createAppStore();
    const requests: Deferred<SessionGitRefreshResponse>[] = [];
    const client = refreshClient(requests);
    const release = retainGitRefreshScope(client, SESSION);
    let released = false;
    try {
      const attempt = requestGitStatusRefresh(client, store, SESSION, SESSION);
      const pending = statusResponse("pending.txt");
      const event = pending.snapshots[0].payload as GitStatusUpdateEvent;
      event.status.detail_state = "pending";
      event.status.branch = null as unknown as string;
      requests[0].resolve(pending);
      await attempt;

      await vi.advanceTimersByTimeAsync(60_000);
      expect(requests).toHaveLength(2);
      release();
      released = true;

      expect(store.getState().gitStatus.byEnvironmentId[SESSION]).toMatchObject({
        branch: null,
        detail_state: "pending",
      });
      await vi.waitFor(() =>
        expect(store.getState().gitStatus.refreshByEnvironmentRepo?.[SESSION]?.[""]).toMatchObject({
          state: "unavailable",
          error_code: "details_unavailable",
        }),
      );
    } finally {
      if (!released) release();
      vi.useRealTimers();
    }
  });
});

describe("Git status refresh coordinator stale quality responses", () => {
  beforeEach(() => {
    vi.useRealTimers();
  });

  it.each(["unavailable", "loading"] as const)(
    "keeps a newer ready snapshot when a correlated %s response is older",
    async (state) => {
      const store = createAppStore();
      const requests: Deferred<SessionGitRefreshResponse>[] = [];
      const client = refreshClient(requests);
      store.getState().setGitStatus(SESSION, {
        status_state: "ready",
        files_complete: true,
        detail_state: "ready",
        branch: "main",
        remote_branch: null,
        modified: [CURRENT_FILE],
        added: [],
        deleted: [],
        untracked: [],
        renamed: [],
        ahead: 0,
        behind: 0,
        files: { [CURRENT_FILE]: { path: CURRENT_FILE, status: "modified", staged: false } },
        tracker_id: TRACKER_ID,
        tracker_epoch: 1,
        snapshot_revision: 8,
        timestamp: CURRENT_TIMESTAMP,
      });
      const release = retainGitRefreshScope(client, SESSION);
      const attempt = requestGitStatusRefresh(client, store, SESSION, SESSION);

      requests[0].resolve(orderedResponse(7, EARLIER_TIMESTAMP, state));
      await vi.waitFor(() => expect(requests).toHaveLength(2));
      requests[1].resolve(orderedResponse(7, EARLIER_TIMESTAMP, state));
      await attempt;

      expect(store.getState().gitStatus.byEnvironmentId[SESSION]?.files).toHaveProperty(
        CURRENT_FILE,
      );
      expect(store.getState().gitStatus.refreshByEnvironmentId?.[SESSION]).toBeUndefined();
      release();
    },
  );
});
