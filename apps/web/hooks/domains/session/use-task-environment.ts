import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";

import {
  fetchTaskEnvironmentLive,
  resetTaskEnvironment,
  type ContainerLiveStatus,
  type TaskEnvironment,
} from "@/lib/api/domains/task-environment-api";
import { ApiError } from "@/lib/api/client";
import {
  getEnvironmentStatusSnapshot,
  resolveExecutorEnvironmentStatus,
  type EnvironmentStatusSnapshot,
} from "@/components/task/executor-environment-status";

const ACTIVE_POLL_INTERVAL_MS = 3000;
const BACKGROUND_POLL_INTERVAL_MS = 7000;

/**
 * Owns the env+container fetch/poll lifecycle and the reset action so the
 * popover component stays small. Polls more quickly while the popover is open
 * (`active=true`) and less frequently while closed so the toolbar icon can
 * still reflect externally stopped/restarted containers.
 */
export function useTaskEnvironment(taskId: string | null | undefined, active: boolean) {
  const [env, setEnv] = useState<TaskEnvironment | null>(null);
  const [container, setContainer] = useState<ContainerLiveStatus | null>(null);
  const [loading, setLoading] = useState(false);
  const [isResetting, setIsResetting] = useState(false);
  const inFlight = useRef(false);
  const lastStatusRef = useRef<EnvironmentStatusSnapshot | null>(null);
  // hasLoadedRef tracks "have we ever fetched successfully" so the spinner
  // only shows on the first open. Keeping it in a ref instead of state means
  // `loadEnv` doesn't depend on `env` — without that, every successful fetch
  // creates a new `env` reference, forces a new `loadEnv` identity, and the
  // polling effect's cleanup+rerun fires immediately, turning the 3-second
  // poll into a tight loop.
  const hasLoadedRef = useRef(false);

  useEffect(() => {
    hasLoadedRef.current = false;
    lastStatusRef.current = null;
    setEnv(null);
    setContainer(null);
    setLoading(false);
  }, [taskId]);

  const updateState = useCallback(
    (nextEnv: TaskEnvironment | null, nextContainer: ContainerLiveStatus | null) => {
      const nextStatus = getEnvironmentStatusSnapshot(nextEnv, nextContainer);
      maybeNotifyEnvironmentStatus(lastStatusRef.current, nextStatus);
      lastStatusRef.current = nextStatus;
      setEnv(nextEnv);
      setContainer(nextContainer);
    },
    [],
  );

  const loadEnv = useCallback(async () => {
    if (!taskId || inFlight.current) return;
    inFlight.current = true;
    setLoading((prev) => prev || (active && !hasLoadedRef.current));
    try {
      const data = await fetchTaskEnvironmentLive(taskId);
      hasLoadedRef.current = true;
      updateState(data.environment, data.container ?? null);
    } catch (err) {
      // Only treat 404 as "no environment yet" — a transient 500 / auth /
      // network error should leave the last-known view in place rather than
      // erase a valid environment and disable the Reset action.
      if (err instanceof ApiError && err.status === 404) {
        hasLoadedRef.current = true;
        updateState(null, null);
      }
    } finally {
      inFlight.current = false;
      setLoading(false);
    }
  }, [active, taskId, updateState]);

  useEffect(() => {
    if (!taskId) return;
    void loadEnv();
    const intervalMs = active ? ACTIVE_POLL_INTERVAL_MS : BACKGROUND_POLL_INTERVAL_MS;
    const interval = window.setInterval(() => void loadEnv(), intervalMs);
    return () => window.clearInterval(interval);
  }, [active, taskId, loadEnv]);

  const reset = useCallback(
    async ({ pushBranch }: { pushBranch: boolean }) => {
      if (!taskId) return false;
      setIsResetting(true);
      try {
        await resetTaskEnvironment(taskId, { push_branch: pushBranch });
        toast.success("Environment reset");
        lastStatusRef.current = getEnvironmentStatusSnapshot(null, null);
        setEnv(null);
        setContainer(null);
        return true;
      } catch (err) {
        const msg = err instanceof Error ? err.message : "Unknown error";
        toast.error(`Reset failed: ${msg}`);
        return false;
      } finally {
        setIsResetting(false);
      }
    },
    [taskId],
  );

  const status = useMemo(
    () => (env ? resolveExecutorEnvironmentStatus(env, container) : null),
    [env, container],
  );

  return { env, container, loading, isResetting, reset, status };
}

function maybeNotifyEnvironmentStatus(
  prev: EnvironmentStatusSnapshot | null,
  next: EnvironmentStatusSnapshot,
) {
  if (!prev || prev.key === next.key) return;
  if (prev.tone === "running" && next.tone !== "running") {
    toast.error("Executor environment stopped", {
      description: `Current state: ${next.label}`,
    });
  } else if (prev.tone !== "running" && next.tone === "running") {
    toast.success("Executor environment running");
  }
}
