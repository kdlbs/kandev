import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { toast } from "@/lib/toast/sonner";
import { t } from "@/lib/i18n";

import {
  fetchTaskEnvironmentLive,
  resetTaskEnvironment,
  type ContainerLiveStatus,
  type SSHLiveStatus,
  type TaskEnvironment,
} from "@/lib/api/domains/task-environment-api";
import { ApiError } from "@/lib/api/client";
import { getKubernetesTaskSession } from "@/lib/api/domains/kubernetes-api";
import {
  getEnvironmentStatusSnapshot,
  resolveExecutorEnvironmentStatus,
  type EnvironmentStatusSnapshot,
  type KubernetesEnvironmentStatus,
} from "@/components/task/executor-environment-status";

const ACTIVE_POLL_INTERVAL_MS = 3000;
const BACKGROUND_POLL_INTERVAL_MS = 7000;

type LiveEnvironmentState = {
  env: TaskEnvironment | null;
  container: ContainerLiveStatus | null;
  ssh: SSHLiveStatus | null;
  kubernetes: KubernetesEnvironmentStatus;
};

const EMPTY_ENVIRONMENT_STATE: LiveEnvironmentState = {
  env: null,
  container: null,
  ssh: null,
  kubernetes: { session: null, loaded: false, error: null },
};

/**
 * Owns the env+container fetch/poll lifecycle and the reset action so the
 * popover component stays small. Polls more quickly while the popover is open
 * (`active=true`) and less frequently while closed so the toolbar icon can
 * still reflect externally stopped/restarted containers.
 */
export function useTaskEnvironment(
  taskId: string | null | undefined,
  sessionId: string | null | undefined,
  active: boolean,
) {
  const { state, loading, refreshing, refresh, clear } = useLiveEnvironment(
    taskId,
    sessionId,
    active,
  );
  const { isResetting, reset } = useEnvironmentReset(taskId, clear);
  const { env, container, ssh } = state;
  const {
    session: kubernetes,
    loaded: kubernetesLoaded,
    error: kubernetesError,
  } = state.kubernetes;

  const status = useMemo(
    () =>
      env
        ? resolveExecutorEnvironmentStatus(
            env,
            container,
            env.executor_type === "k8s" ? state.kubernetes : undefined,
          )
        : null,
    [container, env, state.kubernetes],
  );

  return {
    env,
    container,
    ssh,
    kubernetes,
    kubernetesLoaded,
    kubernetesError,
    loading,
    refreshing,
    isResetting,
    reset,
    refresh,
    status,
  };
}

function useLiveEnvironment(
  taskId: string | null | undefined,
  sessionId: string | null | undefined,
  active: boolean,
) {
  const [state, setState] = useState<LiveEnvironmentState>(EMPTY_ENVIRONMENT_STATE);
  const [loading, setLoading] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const requestScope = `${taskId ?? ""}\u0000${sessionId ?? ""}`;
  const currentScopeRef = useRef(requestScope);
  currentScopeRef.current = requestScope;
  const inFlightRequests = useRef(new Map<string, Promise<void>>());
  const manualRefreshGenerationRef = useRef(0);
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
    setState(EMPTY_ENVIRONMENT_STATE);
    setLoading(false);
    setRefreshing(false);
    manualRefreshGenerationRef.current += 1;
  }, [requestScope]);

  const publish = useCallback((next: LiveEnvironmentState) => {
    const kubernetes = next.env?.executor_type === "k8s" ? next.kubernetes : undefined;
    const nextStatus = getEnvironmentStatusSnapshot(next.env, next.container, kubernetes);
    maybeNotifyEnvironmentStatus(lastStatusRef.current, nextStatus);
    lastStatusRef.current = nextStatus;
    setState(next);
  }, []);

  const loadEnv = useCallback((): Promise<void> => {
    if (!taskId) return Promise.resolve();
    const existing = inFlightRequests.current.get(requestScope);
    if (existing) return existing;

    const request = (async () => {
      setLoading((prev) => prev || (active && !hasLoadedRef.current));
      try {
        const next = await fetchLiveEnvironment(taskId, sessionId);
        if (currentScopeRef.current !== requestScope) return;
        hasLoadedRef.current = true;
        publish(next);
      } catch (err) {
        // Only treat 404 as "no environment yet" — a transient 500 / auth /
        // network error should leave the last-known view in place rather than
        // erase a valid environment and disable the Reset action.
        if (
          err instanceof ApiError &&
          err.status === 404 &&
          currentScopeRef.current === requestScope
        ) {
          hasLoadedRef.current = true;
          publish(EMPTY_ENVIRONMENT_STATE);
        }
      } finally {
        inFlightRequests.current.delete(requestScope);
        if (currentScopeRef.current === requestScope) setLoading(false);
      }
    })();
    inFlightRequests.current.set(requestScope, request);
    return request;
  }, [active, publish, requestScope, sessionId, taskId]);

  const refresh = useCallback(async () => {
    if (!taskId) return;
    const generation = ++manualRefreshGenerationRef.current;
    setRefreshing(true);
    try {
      await loadEnv();
    } finally {
      if (
        generation === manualRefreshGenerationRef.current &&
        currentScopeRef.current === requestScope
      ) {
        setRefreshing(false);
      }
    }
  }, [loadEnv, requestScope, taskId]);

  useEffect(() => {
    if (!taskId) return;
    void loadEnv();
    const intervalMs = active ? ACTIVE_POLL_INTERVAL_MS : BACKGROUND_POLL_INTERVAL_MS;
    const interval = window.setInterval(() => void loadEnv(), intervalMs);
    return () => window.clearInterval(interval);
  }, [active, taskId, loadEnv]);

  const clear = useCallback(() => {
    lastStatusRef.current = getEnvironmentStatusSnapshot(null, null);
    setState(EMPTY_ENVIRONMENT_STATE);
  }, []);

  return { state, loading, refreshing, refresh, clear };
}

async function fetchLiveEnvironment(
  taskId: string,
  sessionId: string | null | undefined,
): Promise<LiveEnvironmentState> {
  const data = await fetchTaskEnvironmentLive(taskId);
  const kubernetes: KubernetesEnvironmentStatus = {
    session: null,
    loaded: data.environment.executor_type !== "k8s",
    error: null,
  };
  if (data.environment.executor_type === "k8s") {
    kubernetes.loaded = true;
    if (sessionId && data.environment.executor_id) {
      try {
        kubernetes.session = await getKubernetesTaskSession(
          data.environment.executor_id,
          taskId,
          sessionId,
        );
      } catch {
        kubernetes.error = t("executors:kubernetesSessionsFailed");
      }
    }
  }
  return {
    env: data.environment,
    container: data.container ?? null,
    ssh: data.ssh ?? null,
    kubernetes,
  };
}

function useEnvironmentReset(taskId: string | null | undefined, clear: () => void) {
  const [isResetting, setIsResetting] = useState(false);

  const reset = useCallback(
    async ({ pushBranch }: { pushBranch: boolean }) => {
      if (!taskId) return false;
      setIsResetting(true);
      try {
        await resetTaskEnvironment(taskId, { push_branch: pushBranch });
        toast.success(t("task:environmentReset"));
        clear();
        return true;
      } catch (err) {
        const msg = err instanceof Error ? err.message : t("task:unknownError");
        toast.error(t("task:environmentResetFailed", { error: msg }));
        return false;
      } finally {
        setIsResetting(false);
      }
    },
    [clear, taskId],
  );
  return { isResetting, reset };
}

function maybeNotifyEnvironmentStatus(
  prev: EnvironmentStatusSnapshot | null,
  next: EnvironmentStatusSnapshot,
) {
  if (!prev || prev.key === next.key) return;
  if (prev.tone === "running" && next.tone !== "running") {
    toast.error(t("task:executorEnvironmentStopped"), {
      description: t("task:environmentStateCurrent", { state: next.label }),
    });
  } else if (prev.tone !== "running" && next.tone === "running") {
    toast.success(t("task:executorEnvironmentRunning"));
  }
}
