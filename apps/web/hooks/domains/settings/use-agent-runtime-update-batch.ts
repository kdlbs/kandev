"use client";

import { useCallback, useEffect, useMemo, useRef } from "react";

import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  getRuntimeUpdateBatchCoordinator,
  recoverAgentUpdateJobs,
  type RuntimeBatchTarget,
} from "@/lib/agents/runtime-update-batch";

/**
 * Narrow adapter over the store-owned bulk coordinator. Renderers must not own
 * submission lifetime: closing the manager or leaving the route leaves the
 * approved run inside the app store.
 */
export function useAgentRuntimeUpdateBatch() {
  const store = useAppStoreApi();
  const batch = useAppStore((state) => state.runtimeUpdateBatch);
  const updateJobs = useAppStore((state) => state.updateJobs.byAgent);
  const connectionStatus = useAppStore((state) => state.connection.status);
  const previousConnection = useRef(connectionStatus);
  const coordinator = useMemo(() => getRuntimeUpdateBatchCoordinator(store), [store]);

  useEffect(() => {
    void recoverAgentUpdateJobs(store);
  }, [store]);

  useEffect(() => {
    if (connectionStatus === "connected" && previousConnection.current !== "connected") {
      void recoverAgentUpdateJobs(store, true);
    }
    previousConnection.current = connectionStatus;
  }, [connectionStatus, store]);

  useEffect(() => {
    const reconnect = () => void recoverAgentUpdateJobs(store, true);
    window.addEventListener("online", reconnect);
    return () => window.removeEventListener("online", reconnect);
  }, [store]);

  useEffect(() => {
    coordinator.observeJobs(updateJobs);
  }, [coordinator, updateJobs]);

  return {
    batch,
    beginReview: useCallback(
      (targets: RuntimeBatchTarget[]) => coordinator.beginReview(targets),
      [coordinator],
    ),
    beginRetryReview: useCallback(
      (targets: RuntimeBatchTarget[]) => coordinator.beginRetryReview(targets),
      [coordinator],
    ),
    start: useCallback(
      (keys: ReadonlySet<string>, eligibleKeys?: ReadonlySet<string>) =>
        coordinator.start(keys, eligibleKeys),
      [coordinator],
    ),
    retryFailed: useCallback(() => coordinator.retryFailed(), [coordinator]),
    refreshStatus: useCallback(() => coordinator.refreshStatus(), [coordinator]),
  };
}
