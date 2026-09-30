import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
  type MutableRefObject,
} from "react";
import { toast } from "@/lib/toast/sonner";
import { t } from "@/lib/i18n";
import { useOptionalAppStoreApi } from "@/components/state-provider";

import {
  resetTaskEnvironment,
  type ContainerLiveStatus,
  type PluginExecutorLiveStatus,
  type SSHLiveStatus,
  type TaskEnvironment,
  type TaskEnvironmentLiveResponse,
} from "@/lib/api/domains/task-environment-api";
import { getKubernetesTaskSession } from "@/lib/api/domains/kubernetes-api";
import { environmentLiveResource } from "./environment-live-resource";
import {
  getEnvironmentStatusSnapshot,
  resolveExecutorEnvironmentStatus,
  type EnvironmentStatusSnapshot,
  type KubernetesEnvironmentStatus,
} from "@/components/task/executor-environment-status";

type LiveEnvironmentState = {
  env: TaskEnvironment | null;
  container: ContainerLiveStatus | null;
  ssh: SSHLiveStatus | null;
  pluginExecutor: PluginExecutorLiveStatus | null;
  kubernetes: KubernetesEnvironmentStatus;
};

type ScopedEnvironmentState = { scope: string; value: LiveEnvironmentState };

type PendingEnvironmentEnrichment = {
  response: TaskEnvironmentLiveResponse;
  owner: ReturnType<typeof useOptionalAppStoreApi>;
  generation: number;
  isCurrent: () => boolean;
  promise: Promise<void>;
};

const EMPTY_ENVIRONMENT_STATE: LiveEnvironmentState = {
  env: null,
  container: null,
  ssh: null,
  pluginExecutor: null,
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
  const { env, container, ssh, pluginExecutor } = state;
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
            pluginExecutor,
          )
        : null,
    [container, env, pluginExecutor, state.kubernetes],
  );

  return {
    env,
    container,
    ssh,
    pluginExecutor,
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
  const storeOwner = useOptionalAppStoreApi();
  const consumerId = useRef(Symbol("task-environment-consumer"));
  const requestScope = `${taskId ?? ""}\u0000${sessionId ?? ""}`;
  const [scopedState, setScopedState] = useState<ScopedEnvironmentState>({
    scope: requestScope,
    value: EMPTY_ENVIRONMENT_STATE,
  });
  const [refreshing, setRefreshing] = useState(false);
  const currentScopeRef = useRef(requestScope);
  const manualRefreshGenerationRef = useRef(0);
  const enrichmentGenerationRef = useRef(0);
  const pendingEnrichmentRef = useRef<PendingEnvironmentEnrichment | null>(null);
  const lastStatusRef = useRef<EnvironmentStatusSnapshot | null>(null);
  const hasLoadedRef = useRef(false);
  const snapshot = useEnvironmentLiveSnapshot(storeOwner, taskId, consumerId, active);
  const state = scopedState.scope === requestScope ? scopedState.value : EMPTY_ENVIRONMENT_STATE;
  currentScopeRef.current = requestScope;

  const publish = useEnvironmentPublisher(lastStatusRef, requestScope, setScopedState);

  const loadEnrichedSnapshot = useEnrichedEnvironmentSnapshot({
    currentScopeRef,
    enrichmentGenerationRef,
    hasLoadedRef,
    pendingEnrichmentRef,
    publish,
    requestScope,
    sessionId,
    storeOwner,
    taskId,
  });

  useEffect(() => {
    hasLoadedRef.current = false;
    lastStatusRef.current = null;
    enrichmentGenerationRef.current += 1;
    pendingEnrichmentRef.current = null;
    setScopedState({ scope: requestScope, value: EMPTY_ENVIRONMENT_STATE });
    setRefreshing(false);
    manualRefreshGenerationRef.current += 1;
  }, [requestScope]);

  useEffect(() => {
    if (!taskId || currentScopeRef.current !== requestScope) return;
    if (snapshot.notFound) {
      hasLoadedRef.current = true;
      publish(EMPTY_ENVIRONMENT_STATE);
      return;
    }
    if (!snapshot.response) return;

    let cancelled = false;
    void loadEnrichedSnapshot(snapshot.response, () => !cancelled);
    return () => {
      cancelled = true;
    };
  }, [loadEnrichedSnapshot, publish, requestScope, snapshot.notFound, snapshot.response, taskId]);

  const refresh = useCallback(async () => {
    if (!taskId) return;
    const generation = ++manualRefreshGenerationRef.current;
    setRefreshing(true);
    try {
      const currentEnrichment = pendingEnrichmentRef.current;
      if (currentEnrichment?.response === snapshot.response) {
        await currentEnrichment.promise;
        return;
      }
      await environmentLiveResource.refresh(storeOwner, taskId);
      const response = environmentLiveResource.getSnapshot(storeOwner, taskId).response;
      if (response) await loadEnrichedSnapshot(response, () => true);
    } finally {
      if (
        generation === manualRefreshGenerationRef.current &&
        currentScopeRef.current === requestScope
      ) {
        setRefreshing(false);
      }
    }
  }, [loadEnrichedSnapshot, requestScope, snapshot.response, storeOwner, taskId]);

  const clear = useCallback(() => {
    lastStatusRef.current = getEnvironmentStatusSnapshot(null, null);
    setScopedState({ scope: requestScope, value: EMPTY_ENVIRONMENT_STATE });
    environmentLiveResource.invalidate(storeOwner, taskId);
  }, [requestScope, storeOwner, taskId]);

  return {
    state,
    loading: active && snapshot.loading && !hasLoadedRef.current,
    refreshing,
    refresh,
    clear,
  };
}

function useEnvironmentPublisher(
  lastStatusRef: MutableRefObject<EnvironmentStatusSnapshot | null>,
  requestScope: string,
  setScopedState: (value: ScopedEnvironmentState) => void,
) {
  return useCallback(
    (next: LiveEnvironmentState) => {
      const kubernetes = next.env?.executor_type === "k8s" ? next.kubernetes : undefined;
      const nextStatus = getEnvironmentStatusSnapshot(
        next.env,
        next.container,
        kubernetes,
        next.pluginExecutor,
      );
      maybeNotifyEnvironmentStatus(lastStatusRef.current, nextStatus);
      lastStatusRef.current = nextStatus;
      setScopedState({ scope: requestScope, value: next });
    },
    [lastStatusRef, requestScope, setScopedState],
  );
}

function useEnrichedEnvironmentSnapshot({
  currentScopeRef,
  enrichmentGenerationRef,
  hasLoadedRef,
  pendingEnrichmentRef,
  publish,
  requestScope,
  sessionId,
  storeOwner,
  taskId,
}: {
  currentScopeRef: MutableRefObject<string>;
  enrichmentGenerationRef: MutableRefObject<number>;
  hasLoadedRef: MutableRefObject<boolean>;
  pendingEnrichmentRef: MutableRefObject<PendingEnvironmentEnrichment | null>;
  publish: (next: LiveEnvironmentState) => void;
  requestScope: string;
  sessionId: string | null | undefined;
  storeOwner: ReturnType<typeof useOptionalAppStoreApi>;
  taskId: string | null | undefined;
}) {
  return useCallback(
    (response: TaskEnvironmentLiveResponse, isCurrent: () => boolean): Promise<void> => {
      if (!taskId) return Promise.resolve();
      const existing = pendingEnrichmentRef.current;
      if (existing?.response === response && existing.owner === storeOwner) {
        existing.isCurrent = isCurrent;
        return existing.promise;
      }

      const generation = ++enrichmentGenerationRef.current;
      const promise = fetchLiveEnvironment(response, taskId, sessionId)
        .then((next) => {
          const current = pendingEnrichmentRef.current;
          if (
            current?.generation !== generation ||
            !current.isCurrent() ||
            currentScopeRef.current !== requestScope ||
            environmentLiveResource.getSnapshot(storeOwner, taskId).response !== response
          ) {
            return;
          }
          hasLoadedRef.current = true;
          publish(next);
        })
        .finally(() => {
          if (pendingEnrichmentRef.current?.generation === generation) {
            pendingEnrichmentRef.current = null;
          }
        });
      pendingEnrichmentRef.current = {
        response,
        owner: storeOwner,
        generation,
        isCurrent,
        promise,
      };
      return promise;
    },
    [
      currentScopeRef,
      enrichmentGenerationRef,
      hasLoadedRef,
      pendingEnrichmentRef,
      publish,
      requestScope,
      sessionId,
      storeOwner,
      taskId,
    ],
  );
}

function useEnvironmentLiveSnapshot(
  storeOwner: ReturnType<typeof useOptionalAppStoreApi>,
  taskId: string | null | undefined,
  consumerId: MutableRefObject<symbol>,
  active: boolean,
) {
  const subscribe = useCallback(
    (listener: () => void) =>
      taskId
        ? environmentLiveResource.subscribe(
            storeOwner,
            taskId,
            consumerId.current,
            listener,
            active,
          )
        : () => undefined,
    [active, storeOwner, taskId],
  );
  const getSnapshot = useCallback(
    () => environmentLiveResource.getSnapshot(storeOwner, taskId),
    [storeOwner, taskId],
  );
  useEffect(() => {
    if (!taskId) return;
    environmentLiveResource.setActive(storeOwner, taskId, consumerId.current, active);
  }, [active, storeOwner, taskId]);
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}

async function fetchLiveEnvironment(
  data: TaskEnvironmentLiveResponse,
  taskId: string,
  sessionId: string | null | undefined,
): Promise<LiveEnvironmentState> {
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
    pluginExecutor: data.plugin_executor ?? null,
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
