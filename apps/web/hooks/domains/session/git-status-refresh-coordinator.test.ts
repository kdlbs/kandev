import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { WebSocketClient, SessionGitRefreshResponse } from "@/lib/ws/client";
import type { GitStatusUpdateEvent } from "@/lib/types/git-events";
import { createAppStore, type AppState } from "@/lib/state/store";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import {
  monitorGitStatusDetails,
  requestGitStatusRefresh,
  retainGitRefreshScope,
} from "./git-status-refresh-coordinator";

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

function unavailableResponse(
  sessionId = SESSION,
  environmentId = sessionId,
): SessionGitRefreshResponse {
  return {
    success: false,
    session_id: sessionId,
    task_environment_id: environmentId,
    mode: "fresh",
    status_state: "unavailable",
    snapshots: [],
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

describe("Git status refresh coordinator delayed recovery", () => {
  beforeEach(() => {
    vi.useRealTimers();
  });

  it("starts one automatic read after the first bounded backoff delay", async () => {
    vi.useFakeTimers();
    const store = createAppStore();
    const requests: Deferred<SessionGitRefreshResponse>[] = [];
    const client = refreshClient(requests);
    const release = retainGitRefreshScope(client, SESSION);
    const releaseSibling = retainGitRefreshScope(client, SESSION);
    const stopMonitoring = monitorGitStatusDetails(client, store, SESSION);
    const stopSiblingMonitoring = monitorGitStatusDetails(client, store, SESSION);

    try {
      const firstAttempt = requestGitStatusRefresh(client, store, SESSION, SESSION);
      const siblingAttempt = requestGitStatusRefresh(client, store, SESSION, SESSION);
      expect(siblingAttempt).toBe(firstAttempt);
      requests[0].resolve(unavailableResponse());
      await vi.waitFor(() => expect(requests).toHaveLength(2));
      requests[1].resolve(unavailableResponse());
      await firstAttempt;

      await vi.advanceTimersByTimeAsync(4_999);
      expect(requests).toHaveLength(2);
      await vi.advanceTimersByTimeAsync(1);
      await vi.waitFor(() => expect(requests).toHaveLength(3));
      await vi.advanceTimersByTimeAsync(60_000);
      expect(requests).toHaveLength(3);

      const recovered = statusResponse(CURRENT_FILE);
      (recovered.snapshots[0].payload as GitStatusUpdateEvent).timestamp =
        "2026-09-30T10:00:03.000Z";
      requests[2].resolve(recovered);
      await vi.waitFor(() =>
        expect(store.getState().gitStatus.byEnvironmentId[SESSION]?.files).toHaveProperty(
          CURRENT_FILE,
        ),
      );
      expect(store.getState().gitStatus.refreshByEnvironmentId?.[SESSION]).toBeUndefined();
    } finally {
      stopMonitoring();
      stopSiblingMonitoring();
      release();
      releaseSibling();
      vi.useRealTimers();
    }
  });
});

describe("Git status refresh monitor", () => {
  it("does not rewalk file metadata after unrelated store updates", () => {
    const baseState = createAppStore().getState();
    const requests: Deferred<SessionGitRefreshResponse>[] = [];
    const client = refreshClient(requests);
    let fileDetailReads = 0;
    const file = new Proxy(
      { path: CURRENT_FILE, status: "modified", staged: false } as GitStatusEntry["files"][string],
      {
        get(target, property, receiver) {
          if (
            property === "diff_state" ||
            property === "staged_change" ||
            property === "unstaged_change"
          ) {
            fileDetailReads += 1;
          }
          return Reflect.get(target, property, receiver);
        },
      },
    );
    const status: GitStatusEntry = {
      branch: "main",
      remote_branch: null,
      modified: [CURRENT_FILE],
      added: [],
      deleted: [],
      untracked: [],
      renamed: [],
      ahead: 0,
      behind: 0,
      files: { [CURRENT_FILE]: file },
      timestamp: CURRENT_TIMESTAMP,
      status_state: "ready",
      files_complete: true,
      detail_state: "ready",
    };
    let currentState: AppState = {
      ...baseState,
      environmentIdBySessionId: { ...baseState.environmentIdBySessionId, [SESSION]: SESSION },
      gitStatus: {
        ...baseState.gitStatus,
        byEnvironmentId: { ...baseState.gitStatus.byEnvironmentId, [SESSION]: status },
      },
    };
    const listeners = new Set<(state: AppState, previousState: AppState) => void>();
    const store = {
      getState: () => currentState,
      subscribe: (listener: (state: AppState, previousState: AppState) => void) => {
        listeners.add(listener);
        return () => listeners.delete(listener);
      },
    } as unknown as StoreApi<AppState>;
    const updateState = (nextState: AppState) => {
      const previousState = currentState;
      currentState = nextState;
      for (const listener of listeners) listener(currentState, previousState);
    };
    const release = retainGitRefreshScope(client, SESSION);
    const stopMonitoring = monitorGitStatusDetails(client, store, SESSION);

    try {
      updateState({
        ...currentState,
        tasks: { ...currentState.tasks, activeTaskId: "another-task" },
      });
      expect(fileDetailReads).toBe(0);

      updateState({
        ...currentState,
        gitStatus: {
          ...currentState.gitStatus,
          byEnvironmentId: {
            ...currentState.gitStatus.byEnvironmentId,
            [SESSION]: { ...status, timestamp: "2026-09-30T10:00:03.000Z" },
          },
        },
      });
      expect(fileDetailReads).toBeGreaterThan(0);
    } finally {
      stopMonitoring();
      release();
    }
  });
});

describe("Git status refresh recovery backoff", () => {
  beforeEach(() => {
    vi.useRealTimers();
  });

  it("uses increasing recovery delays and caps the delay at thirty seconds", async () => {
    vi.useFakeTimers();
    const store = createAppStore();
    const requests: Deferred<SessionGitRefreshResponse>[] = [];
    const client = refreshClient(requests);
    const release = retainGitRefreshScope(client, SESSION);
    const stopMonitoring = monitorGitStatusDetails(client, store, SESSION);

    try {
      const firstAttempt = requestGitStatusRefresh(client, store, SESSION, SESSION);
      requests[0].resolve(unavailableResponse());
      await vi.waitFor(() => expect(requests).toHaveLength(2));
      requests[1].resolve(unavailableResponse());
      await firstAttempt;

      const delays = [5_000, 10_000, 20_000, 30_000, 30_000];
      let nextAttemptStart = 2;
      for (const [index, delay] of delays.entries()) {
        await vi.advanceTimersByTimeAsync(delay - 1);
        expect(requests).toHaveLength(nextAttemptStart);
        await vi.advanceTimersByTimeAsync(1);
        await vi.waitFor(() => expect(requests).toHaveLength(nextAttemptStart + 1));
        if (index === delays.length - 1) {
          requests[nextAttemptStart].resolve(statusResponse(CURRENT_FILE));
          await vi.waitFor(() =>
            expect(store.getState().gitStatus.byEnvironmentId[SESSION]?.files).toHaveProperty(
              CURRENT_FILE,
            ),
          );
          break;
        }
        requests[nextAttemptStart].resolve(unavailableResponse());
        await vi.waitFor(() => expect(requests).toHaveLength(nextAttemptStart + 2));
        requests[nextAttemptStart + 1].resolve(unavailableResponse());
        await vi.advanceTimersByTimeAsync(0);
        nextAttemptStart += 2;
      }
      expect(store.getState().gitStatus.refreshByEnvironmentId?.[SESSION]).toBeUndefined();
    } finally {
      stopMonitoring();
      release();
      vi.useRealTimers();
    }
  });
});

describe("Git status refresh recovery cancellation", () => {
  beforeEach(() => {
    vi.useRealTimers();
  });

  it("cancels backoff on accepted recovery and starts a new scope at the first delay", async () => {
    vi.useFakeTimers();
    const store = createAppStore();
    const requests: Deferred<SessionGitRefreshResponse>[] = [];
    const client = refreshClient(requests);
    const release = retainGitRefreshScope(client, SESSION);
    const stopMonitoring = monitorGitStatusDetails(client, store, SESSION);

    try {
      const firstAttempt = requestGitStatusRefresh(client, store, SESSION, SESSION);
      requests[0].resolve(unavailableResponse());
      await vi.waitFor(() => expect(requests).toHaveLength(2));
      requests[1].resolve(unavailableResponse());
      await firstAttempt;

      store.getState().setGitStatus(SESSION, {
        status_state: "ready",
        files_complete: true,
        detail_state: "ready",
        branch: "main",
        remote_branch: null,
        modified: [],
        added: [],
        deleted: [],
        untracked: [],
        renamed: [],
        ahead: 0,
        behind: 0,
        files: {},
        timestamp: CURRENT_TIMESTAMP,
      });
      store.getState().setGitStatusRefresh(SESSION, undefined, null);
      await vi.advanceTimersByTimeAsync(5_000);
      expect(requests).toHaveLength(2);

      store.getState().setGitStatusRefresh(SESSION, undefined, {
        state: "unavailable",
        error_code: "status_timeout",
      });
      await vi.advanceTimersByTimeAsync(4_999);
      expect(requests).toHaveLength(2);
      await vi.advanceTimersByTimeAsync(1);
      await vi.waitFor(() => expect(requests).toHaveLength(3));
      const recovered = statusResponse(CURRENT_FILE);
      (recovered.snapshots[0].payload as GitStatusUpdateEvent).timestamp =
        "2026-09-30T10:00:03.000Z";
      requests[2].resolve(recovered);
      await vi.waitFor(() =>
        expect(store.getState().gitStatus.byEnvironmentId[SESSION]?.files).toHaveProperty(
          CURRENT_FILE,
        ),
      );
    } finally {
      stopMonitoring();
      release();
      vi.useRealTimers();
    }
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
