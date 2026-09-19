import { getKubernetesTaskSession } from "@/lib/api/domains/kubernetes-api";
import { t } from "@/lib/i18n";
import type { KubernetesSession } from "@/lib/types/http-kubernetes";
import { getWebSocketClient } from "@/lib/ws/connection";

export type RemoteExecutorStatusData = {
  remote_name?: string;
  remote_state?: string;
  remote_created_at?: string;
  remote_checked_at?: string;
  remote_status_error?: string;
  remote_restarts?: number;
  remote_workspace_kind?: string;
};

export type RemoteExecutorStatusRequest = {
  executorId?: string | null;
  executorType?: string | null;
  taskId: string;
  sessionId: string;
};

export type RemoteExecutorStatusSnapshot = {
  status: RemoteExecutorStatusData | null;
  loading: boolean;
};

type Requester = (request: RemoteExecutorStatusRequest) => Promise<RemoteExecutorStatusData> | null;

type Entry = {
  snapshot: RemoteExecutorStatusSnapshot;
  promise: Promise<RemoteExecutorStatusSnapshot> | null;
  listeners: Set<() => void>;
  completedAt: number;
  failed: boolean;
  lastUsed: number;
};

const EMPTY_SNAPSHOT: RemoteExecutorStatusSnapshot = { status: null, loading: false };
const SUCCESS_FRESHNESS_MS = 90_000;
const MAX_ENTRIES = 128;

function projectKubernetesStatus(session: KubernetesSession | null): RemoteExecutorStatusData {
  return {
    remote_name: session?.pod_name,
    remote_state: session?.container_state ?? session?.pod_phase ?? "missing",
    remote_created_at: session?.created_at,
    remote_checked_at: new Date().toISOString(),
    remote_status_error: session?.failure_reason,
    remote_restarts: session?.restarts,
    remote_workspace_kind: session?.workspace_kind,
  };
}

function statusError(): RemoteExecutorStatusData {
  return {
    remote_checked_at: new Date().toISOString(),
    remote_status_error: t("task:remoteExecutorStatusUnavailable"),
  };
}

function publicRemoteStatus(
  request: RemoteExecutorStatusRequest,
  status: RemoteExecutorStatusData,
): RemoteExecutorStatusData {
  if (request.executorType === "k8s" || !status.remote_status_error) return status;
  return {
    ...status,
    remote_status_error: t("task:remoteExecutorStatusUnavailable"),
  };
}

function requestRemoteStatus(
  request: RemoteExecutorStatusRequest,
): Promise<RemoteExecutorStatusData> | null {
  const { executorId, executorType, taskId, sessionId } = request;
  if (executorType === "k8s" && executorId) {
    return getKubernetesTaskSession(executorId, taskId, sessionId).then(projectKubernetesStatus);
  }
  return (
    getWebSocketClient()?.request<RemoteExecutorStatusData>(
      "task.session.status",
      { task_id: taskId, session_id: sessionId },
      10000,
    ) ?? null
  );
}

export function remoteExecutorStatusScope(request: RemoteExecutorStatusRequest): string {
  return JSON.stringify([
    request.executorType ?? "",
    request.executorId ?? "",
    request.taskId,
    request.sessionId,
  ]);
}

export function isValidRemoteExecutorStatusRequest(request: RemoteExecutorStatusRequest): boolean {
  if (!request.taskId || !request.sessionId) return false;
  return request.executorType !== "k8s" || Boolean(request.executorId);
}

function notify(entry: Entry): void {
  entry.listeners.forEach((listener) => listener());
}

export function createRemoteExecutorStatusResource(requester: Requester = requestRemoteStatus) {
  const entries = new Map<string, Entry>();
  let usageCounter = 0;

  function touch(entry: Entry): void {
    entry.lastUsed = ++usageCounter;
  }

  function prune(current: Entry): void {
    const candidates = [...entries.entries()]
      .filter(([, entry]) => entry !== current && !entry.promise && entry.listeners.size === 0)
      .sort((left, right) => left[1].lastUsed - right[1].lastUsed);
    for (const [scope] of candidates) {
      if (entries.size <= MAX_ENTRIES) break;
      entries.delete(scope);
    }
  }

  function entryFor(scope: string): Entry {
    const existing = entries.get(scope);
    if (existing) {
      touch(existing);
      return existing;
    }
    const entry: Entry = {
      snapshot: EMPTY_SNAPSHOT,
      promise: null,
      listeners: new Set(),
      completedAt: 0,
      failed: false,
      lastUsed: 0,
    };
    entries.set(scope, entry);
    touch(entry);
    return entry;
  }

  function load(
    request: RemoteExecutorStatusRequest,
    force = false,
  ): Promise<RemoteExecutorStatusSnapshot> | null {
    if (!isValidRemoteExecutorStatusRequest(request)) return null;
    const entry = entryFor(remoteExecutorStatusScope(request));
    if (entry.promise) return entry.promise;
    const successIsFresh = Date.now() - entry.completedAt < SUCCESS_FRESHNESS_MS;
    if (!force && entry.snapshot.status && !entry.failed && successIsFresh) {
      return Promise.resolve(entry.snapshot);
    }
    const pending = requester(request);
    if (!pending) return null;
    entry.snapshot = { status: entry.snapshot.status, loading: true };
    notify(entry);
    const promise = pending
      .then(
        (status) => ({
          snapshot: { status: publicRemoteStatus(request, status), loading: false },
          failed: Boolean(status.remote_status_error),
        }),
        () => ({
          snapshot: { status: statusError(), loading: false },
          failed: true,
        }),
      )
      .then(({ snapshot, failed }) => {
        entry.snapshot = snapshot;
        entry.failed = failed;
        entry.completedAt = failed ? 0 : Date.now();
        touch(entry);
        notify(entry);
        prune(entry);
        return snapshot;
      })
      .finally(() => {
        if (entry.promise === promise) entry.promise = null;
      });
    entry.promise = promise;
    return promise;
  }

  return {
    getSnapshot(scope: string): RemoteExecutorStatusSnapshot {
      if (!scope) return EMPTY_SNAPSHOT;
      const entry = entries.get(scope);
      if (!entry) return EMPTY_SNAPSHOT;
      touch(entry);
      return entry.snapshot;
    },
    load,
    subscribe(scope: string, listener: () => void): () => void {
      if (!scope) return () => undefined;
      const entry = entryFor(scope);
      entry.listeners.add(listener);
      return () => entry.listeners.delete(listener);
    },
  };
}

export const remoteExecutorStatusResource = createRemoteExecutorStatusResource();
