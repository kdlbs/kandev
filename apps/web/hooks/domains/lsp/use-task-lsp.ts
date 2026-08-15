import { useCallback, useEffect, useMemo } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  getTaskLsp,
  restartTaskLsp,
  setTaskLspPolicy,
  startTaskLsp,
  stopTaskLsp,
} from "@/lib/api/domains/lsp-api";
import type { TaskLspAction, TaskLspPolicy } from "@/lib/types/http-lsp";
import { getWebSocketClient } from "@/lib/ws/connection";

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function controlRequest(action: "start" | "stop" | "restart") {
  if (action === "start") return startTaskLsp;
  if (action === "stop") return stopTaskLsp;
  return restartTaskLsp;
}

const TRANSIENT_REFRESH_MS = 1_500;
const TRANSIENT_PHASES = new Set([
  "waiting_for_task",
  "queued",
  "installing",
  "starting",
  "process_started",
  "initializing",
  "stopping",
]);

type AuthoritativeRequestOrder = { next: number; settled: number };

const authoritativeRequestOrders = new WeakMap<object, Map<string, AuthoritativeRequestOrder>>();

function beginAuthoritativeRequest(store: object, taskId: string): number {
  let byTask = authoritativeRequestOrders.get(store);
  if (!byTask) {
    byTask = new Map();
    authoritativeRequestOrders.set(store, byTask);
  }
  const order = byTask.get(taskId) ?? { next: 0, settled: 0 };
  order.next += 1;
  byTask.set(taskId, order);
  return order.next;
}

function settleAuthoritativeRequest(store: object, taskId: string, request: number): boolean {
  const order = authoritativeRequestOrders.get(store)?.get(taskId);
  if (!order || request < order.settled) return false;
  order.settled = request;
  return true;
}

function taskPending(
  pendingByKey: Record<string, TaskLspAction | undefined>,
  taskId: string | null,
) {
  if (!taskId) return {};
  const prefix = `${taskId}:`;
  return Object.fromEntries(
    Object.entries(pendingByKey)
      .filter(([key, action]) => key.startsWith(prefix) && action)
      .map(([key, action]) => [key.slice(prefix.length), action]),
  );
}

function isLoadedTaskState(task: { loaded: boolean } | undefined): boolean {
  return task?.loaded ?? false;
}

function taskControlErrorEpoch(store: ReturnType<typeof useAppStoreApi>, taskId: string): number {
  return store.getState().taskLsp.byTaskId[taskId]?.controlErrorEpoch ?? 0;
}

function useTaskLspSubscription(
  taskId: string | null,
  connectionStatus: string,
  load: (signal?: AbortSignal) => Promise<void>,
) {
  useEffect(() => {
    if (!taskId) return;
    const client = getWebSocketClient();
    if (!client) return;
    const controller = new AbortController();
    const subscription = client.subscribeTaskWithReady(taskId);
    void subscription.ready.then(() => load(controller.signal)).catch(() => undefined);
    return () => {
      controller.abort();
      subscription.unsubscribe();
    };
  }, [connectionStatus, load, taskId]);
}

function useTransientRefresh(taskId: string | null, loaded: boolean, needed: boolean) {
  const store = useAppStoreApi();
  useEffect(() => {
    if (!taskId || !loaded || !needed) return;
    const controller = new AbortController();
    let inFlight = false;
    const refresh = async () => {
      if (inFlight) return;
      inFlight = true;
      const request = beginAuthoritativeRequest(store, taskId);
      const controlErrorEpoch = taskControlErrorEpoch(store, taskId);
      try {
        const snapshot = await getTaskLsp(taskId, { init: { signal: controller.signal } });
        if (!controller.signal.aborted && settleAuthoritativeRequest(store, taskId, request)) {
          store.getState().setTaskLspSnapshot(snapshot, controlErrorEpoch);
        }
      } catch {
        // WebSocket notifications remain primary. Poll failures must not
        // replace actionable lifecycle evidence with a transport error.
      } finally {
        inFlight = false;
      }
    };
    const timer = window.setInterval(() => void refresh(), TRANSIENT_REFRESH_MS);
    return () => {
      controller.abort();
      window.clearInterval(timer);
    };
  }, [loaded, needed, store, taskId]);
}

export function useTaskLsp(taskId: string | null) {
  const store = useAppStoreApi();
  const task = useAppStore((state) => (taskId ? state.taskLsp.byTaskId[taskId] : undefined));
  const pendingByKey = useAppStore((state) => state.taskLsp.pendingByKey);
  const connectionStatus = useAppStore((state) => state.connection.status);
  const loaded = isLoadedTaskState(task);

  const load = useCallback(
    async (signal?: AbortSignal) => {
      if (!taskId) throw new Error("A task is required to load language servers.");
      const state = store.getState();
      state.setTaskLspLoading(taskId, true);
      const request = beginAuthoritativeRequest(store, taskId);
      const controlErrorEpoch = taskControlErrorEpoch(store, taskId);
      try {
        const snapshot = await getTaskLsp(taskId, { init: { signal } });
        if (!signal?.aborted && settleAuthoritativeRequest(store, taskId, request)) {
          store.getState().setTaskLspSnapshot(snapshot, controlErrorEpoch);
        }
      } catch (error) {
        if (!signal?.aborted && settleAuthoritativeRequest(store, taskId, request)) {
          store.getState().setTaskLspLoadError(taskId, errorMessage(error), controlErrorEpoch);
        }
        if (!signal?.aborted) throw error;
      }
    },
    [store, taskId],
  );

  useTaskLspSubscription(taskId, connectionStatus, load);

  useEffect(() => {
    if (!taskId || loaded) return;
    const controller = new AbortController();
    void load(controller.signal).catch(() => undefined);
    return () => controller.abort();
  }, [load, loaded, taskId]);

  const runControl = useCallback(
    async (language: string, action: Exclude<TaskLspAction, "" | "set_policy" | "reconcile">) => {
      if (!taskId) throw new Error("A task is required to control language servers.");
      const invoke = controlRequest(action);
      store.getState().setTaskLspActionPending(taskId, language, action);
      try {
        const result = await invoke(taskId, language);
        store.getState().mergeTaskLspLanguage(result);
        return result;
      } catch (error) {
        store.getState().setTaskLspError(taskId, errorMessage(error));
        throw error;
      } finally {
        store.getState().setTaskLspActionPending(taskId, language);
      }
    },
    [store, taskId],
  );

  const setPolicy = useCallback(
    async (language: string, policy: TaskLspPolicy) => {
      if (!taskId) throw new Error("A task is required to set language-server policy.");
      store.getState().setTaskLspActionPending(taskId, language, "set_policy");
      try {
        const result = await setTaskLspPolicy(taskId, language, policy);
        store.getState().mergeTaskLspLanguage(result);
        return result;
      } catch (error) {
        store.getState().setTaskLspError(taskId, errorMessage(error));
        throw error;
      } finally {
        store.getState().setTaskLspActionPending(taskId, language);
      }
    },
    [store, taskId],
  );

  const byLanguage = task?.languages ?? {};
  const languages = useMemo(
    () => Object.values(byLanguage).sort((a, b) => a.language.localeCompare(b.language)),
    [byLanguage],
  );
  const pending = useMemo(() => taskPending(pendingByKey, taskId), [pendingByKey, taskId]);

  const needsAuthoritativeRefresh = languages.some(
    (language) =>
      TRANSIENT_PHASES.has(language.phase) ||
      language.activity === "server_work" ||
      language.progress.length > 0,
  );

  useTransientRefresh(taskId, loaded, needsAuthoritativeRefresh);

  return {
    taskId,
    byLanguage,
    languages,
    capacity: task?.capacity ?? { active: 0, queued: 0, limit: 0 },
    loaded,
    loading: task?.loading ?? false,
    error: task?.error ?? null,
    pending,
    refetch: () => load(),
    start: (language: string) => runControl(language, "start"),
    stop: (language: string) => runControl(language, "stop"),
    restart: (language: string) => runControl(language, "restart"),
    setPolicy,
  };
}
