"use client";

import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { useOptionalAppStoreApi } from "@/components/state-provider";
import type { ContainerLiveStatus, TaskEnvironment } from "@/lib/api/domains/task-environment-api";
import { environmentLiveResource } from "./environment-live-resource";
import {
  resolveExecutorEnvironmentStatus,
  type ExecutorEnvironmentStatus,
} from "@/components/task/executor-environment-status";

export function isExecutorEnvironmentUnavailable(
  status: ExecutorEnvironmentStatus | null,
): boolean {
  if (!status) return false;
  if (status.tone === "running" || status.tone === "neutral") return false;
  return status.label !== "starting";
}

export function useExecutorEnvironmentAvailability(taskId: string | null, enabled: boolean) {
  const storeOwner = useOptionalAppStoreApi();
  const consumerId = useRef(Symbol("executor-environment-consumer"));
  const [loadedState, setLoadedState] = useState<{
    taskId: string | null;
    env: TaskEnvironment | null;
    container: ContainerLiveStatus | null;
  }>({ taskId, env: null, container: null });
  const retainedState =
    loadedState.taskId === taskId ? loadedState : { taskId, env: null, container: null };
  const subscribe = useCallback(
    (listener: () => void) =>
      enabled && taskId
        ? environmentLiveResource.subscribe(storeOwner, taskId, consumerId.current, listener)
        : () => undefined,
    [enabled, storeOwner, taskId],
  );
  const getSnapshot = useCallback(
    () => (enabled ? environmentLiveResource.getSnapshot(storeOwner, taskId) : null),
    [enabled, storeOwner, taskId],
  );
  const snapshot = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);

  useEffect(() => {
    setLoadedState({ taskId, env: null, container: null });
  }, [taskId]);

  useEffect(() => {
    if (!enabled || !taskId) return;
    environmentLiveResource.setActive(storeOwner, taskId, consumerId.current, true);
  }, [enabled, storeOwner, taskId]);

  useEffect(() => {
    if (!enabled || !taskId || !snapshot) return;
    if (snapshot.notFound) {
      setLoadedState({ taskId, env: null, container: null });
      return;
    }
    if (snapshot.response) {
      setLoadedState({
        taskId,
        env: snapshot.response.environment,
        container: snapshot.response.container ?? null,
      });
    }
  }, [enabled, snapshot, taskId]);

  const env = enabled && snapshot?.notFound ? null : retainedState.env;
  const container = enabled && snapshot?.notFound ? null : retainedState.container;

  const status = useMemo(
    () => (env ? resolveExecutorEnvironmentStatus(env, container) : null),
    [env, container],
  );
  const unavailable = isExecutorEnvironmentUnavailable(status);

  return { status, unavailable };
}
