"use client";

import { useCallback, useRef, useState } from "react";
import { ApiError } from "@/lib/api/client";
import { retryAgentRuntime } from "@/lib/api/domains/system-api";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import { generateUUID } from "@/lib/utils";

export type AgentRuntimeRecoveryError = "stale" | "failed";

export function useAgentRuntimeRecovery() {
  const agentRuntime = useAppStore((state) => state.agentRuntime);
  const store = useAppStoreApi();
  const isAdmin = useIsAdmin();
  const [isRetrying, setIsRetrying] = useState(false);
  const [error, setError] = useState<AgentRuntimeRecoveryError | null>(null);
  const activeRequest = useRef(false);

  const canRetry =
    isAdmin &&
    agentRuntime?.status === "unavailable" &&
    agentRuntime.retry_allowed === true &&
    Boolean(agentRuntime.boot_id) &&
    typeof agentRuntime.runtime_epoch === "number" &&
    typeof agentRuntime.revision === "number";

  const retry = useCallback(async () => {
    if (
      !isAdmin ||
      agentRuntime?.status !== "unavailable" ||
      agentRuntime.retry_allowed !== true ||
      !agentRuntime.boot_id ||
      typeof agentRuntime.runtime_epoch !== "number" ||
      typeof agentRuntime.revision !== "number" ||
      activeRequest.current
    ) {
      return;
    }
    activeRequest.current = true;
    setIsRetrying(true);
    setError(null);
    try {
      const snapshot = await retryAgentRuntime({
        boot_id: agentRuntime.boot_id,
        runtime_epoch: agentRuntime.runtime_epoch,
        revision: agentRuntime.revision,
        request_id: generateUUID(),
      });
      const current = store.getState().agentRuntime;
      if (!current?.boot_id || current.boot_id === agentRuntime.boot_id) {
        store.getState().setAgentRuntime(snapshot);
      }
    } catch (cause) {
      setError(cause instanceof ApiError && cause.status === 409 ? "stale" : "failed");
    } finally {
      activeRequest.current = false;
      setIsRetrying(false);
    }
  }, [agentRuntime, isAdmin, store]);

  return {
    canRetry,
    error,
    isAdmin,
    isRecovering: agentRuntime?.status === "recovering" || isRetrying,
    isRetrying,
    retry,
  };
}
