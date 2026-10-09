"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type MutableRefObject,
  type SetStateAction,
} from "react";
import { StateHydrator } from "@/components/state-hydrator";
import { useAppStoreApi } from "@/components/state-provider";
import { TaskRouteSessionHydrationProvider } from "@/components/task/task-route-session-hydration";
import { KanbanTaskShell } from "@/app/tasks/[id]/kanban-task-shell";
import {
  buildTaskNavigationShellData,
  extractInitialRepositories,
  extractInitialScripts,
  fetchTaskNavigationEnrichment,
  type FetchedSessionData,
} from "@/lib/ssr/session-page-state";
import { useTranslation } from "react-i18next";
import { isDetachedManagedConversation } from "@/lib/plugins/retained-managed-conversation";
import { RetainedManagedConversationTranscript } from "@/components/plugins/retained-managed-conversation-transcript";
import { captureTaskSessionHydrationEpochs } from "@/lib/state/slices/session/hydration-epochs";
import type { TaskSessionHydrationEpoch } from "@/lib/state/slices/session/types";
import {
  beginTaskNavigation,
  isTaskNavigationCurrent,
  retainTaskNavigationRead,
  readTaskNavigationIdentity,
  useTaskNavigationReadState,
} from "@/lib/state/task-navigation-reads";
import type { TaskNavigationContext } from "@/lib/state/task-navigation-reads";
import { getOwnedTaskSessionId, useTaskRouteProjection } from "./task-route-projection";
import { useTaskDetailRouteRecovery } from "./task-detail-route-recovery";
import { deriveTaskDetailRouteView } from "./task-detail-route-view";
import type { TaskDetailRouteState } from "./task-detail-route-state";

type TaskDetailRouteProps = {
  taskId: string;
  sessionId?: string;
  layout?: string | null;
  simple?: string;
  mode?: string;
  initialData?: FetchedSessionData;
};

function taskRouteKey(taskId: string, sessionId?: string): string {
  return `${taskId}\u0000${sessionId ?? ""}`;
}

function routeDataMatchesTask(
  data: FetchedSessionData | undefined,
  taskId: string,
): data is FetchedSessionData {
  return data?.task?.id === taskId;
}

function routeDataMatchesSelection(
  data: FetchedSessionData | undefined,
  taskId: string,
  sessionId?: string,
): data is FetchedSessionData {
  return routeDataMatchesTask(data, taskId) && (!sessionId || data.sessionId === sessionId);
}

function initialRouteState(
  initialData: FetchedSessionData | undefined,
  taskId: string,
  sessionId?: string,
): TaskDetailRouteState {
  const routeKey = taskRouteKey(taskId, sessionId);
  if (routeDataMatchesSelection(initialData, taskId, sessionId)) {
    return { routeKey, status: "loaded", data: initialData, forceMergeSession: true };
  }
  return { routeKey, status: "loading", data: null };
}

export function TaskDetailRoute({
  taskId,
  sessionId,
  layout,
  simple,
  mode,
  initialData,
}: TaskDetailRouteProps) {
  const [hydratedState, setHydratedState] = useState<FetchedSessionData["initialState"] | null>(
    null,
  );
  const route = useTaskDetailRouteData({ taskId, sessionId, initialData, hydratedState });
  const markRouteHydrated = useCallback(() => {
    setHydratedState(route.initialState);
  }, [route.initialState]);
  const onRouteHydrated =
    route.currentRouteStatus === "loaded" &&
    route.displayedRouteKey === route.routeKey &&
    route.initialState !== null
      ? markRouteHydrated
      : undefined;
  const routeDataReady =
    route.currentRouteStatus === "error" ||
    (route.currentRouteStatus === "loaded" &&
      (route.initialState === null || hydratedState === route.initialState));

  if (route.showInitialLoading) {
    return <TaskRouteLoading retrying={route.readState.phase === "retrying"} />;
  }

  return (
    <TaskDetailRouteBody
      route={route}
      layout={layout}
      simple={simple}
      mode={mode}
      routeDataReady={routeDataReady}
      onRouteHydrated={onRouteHydrated}
    />
  );
}

type TaskDetailRouteView = ReturnType<typeof deriveTaskDetailRouteView> & {
  readState: ReturnType<typeof useTaskNavigationReadState>;
};

function TaskDetailRouteBody({
  route,
  layout,
  simple,
  mode,
  routeDataReady,
  onRouteHydrated,
}: {
  route: TaskDetailRouteView;
  layout?: string | null;
  simple?: string;
  mode?: string;
  routeDataReady: boolean;
  onRouteHydrated?: () => void;
}) {
  return (
    <div className="relative h-full min-h-0 w-full" aria-busy={route.isLoadingOverPreviousRoute}>
      {route.initialState ? (
        <StateHydrator
          initialState={route.initialState}
          sessionId={route.forceMergeSession ? (route.activeSessionId ?? undefined) : undefined}
          taskSessionHydrationEpochsAtRequestStart={route.hydrationEpochsAtRequestStart}
          onHydrated={onRouteHydrated}
        />
      ) : null}
      {route.showShell ? (
        <TaskRouteSessionHydrationProvider isReady={routeDataReady}>
          <div className="h-full min-h-0 w-full" inert={route.isLoadingOverPreviousRoute}>
            {route.task && isDetachedManagedConversation(route.task) ? (
              <RetainedManagedConversationTranscript
                task={route.task}
                sessionId={route.activeSessionId}
              />
            ) : (
              <KanbanTaskShell
                task={route.task}
                taskId={route.shellTaskId}
                sessionId={route.activeSessionId}
                initialRepositories={extractInitialRepositories(route.initialState, route.task)}
                initialScripts={extractInitialScripts(route.initialState, route.task)}
                initialTerminals={route.data?.initialTerminals ?? []}
                defaultLayouts={{}}
                initialLayout={layout}
                urlSimple={simple}
                urlMode={mode}
              />
            )}
          </div>
        </TaskRouteSessionHydrationProvider>
      ) : (
        <TaskRouteLoading />
      )}
      {route.isLoadingOverPreviousRoute ? (
        <TaskRouteLoading overlay retrying={route.readState.phase === "retrying"} />
      ) : null}
    </div>
  );
}

type TaskDetailRouteData = {
  taskId: string;
  sessionId?: string;
  initialData?: FetchedSessionData;
  hydratedState: FetchedSessionData["initialState"] | null;
};

function useTaskDetailRouteData({
  taskId,
  sessionId,
  initialData,
  hydratedState,
}: TaskDetailRouteData) {
  const store = useAppStoreApi();
  const projection = useTaskRouteProjection(taskId, sessionId);
  const routeKey = taskRouteKey(taskId, sessionId);
  const navigationOwnerRef = useRef<object>({});
  const navigationContext = beginTaskNavigation(store, navigationOwnerRef.current, routeKey);
  const selectedSessionForNavigationRef = useRef<{
    navigationContext: TaskNavigationContext;
    sessionId?: string;
  } | null>(null);
  if (selectedSessionForNavigationRef.current?.navigationContext !== navigationContext) {
    const state = store.getState();
    selectedSessionForNavigationRef.current = {
      navigationContext,
      sessionId:
        sessionId ?? getOwnedTaskSessionId(state, taskId, state.tasks.activeSessionId) ?? undefined,
    };
  }
  const selectedSessionForNavigation = selectedSessionForNavigationRef.current.sessionId;
  const readState = useTaskNavigationReadState(store, taskId, navigationContext);
  const appliedRecoveryCycleRef = useRef({ generation: navigationContext.generation, cycle: 0 });
  if (appliedRecoveryCycleRef.current.generation !== navigationContext.generation) {
    appliedRecoveryCycleRef.current = { generation: navigationContext.generation, cycle: 0 };
  }
  const hydrationEpochsAtRequestStartRef = useRef<
    Readonly<Record<string, TaskSessionHydrationEpoch>> | undefined
  >(undefined);
  const bootRouteKeyRef = useRef(routeKey);
  const bootDataConsumedRef = useRef(false);
  if (routeKey !== bootRouteKeyRef.current) bootDataConsumedRef.current = true;
  const routeInitialData = bootDataConsumedRef.current ? undefined : initialData;
  const [routeState, setRouteState] = useState<TaskDetailRouteState>(() =>
    initialRouteState(routeInitialData, taskId, sessionId),
  );
  const previousLoadedRouteRef = useRef<TaskDetailRouteState | null>(
    routeState.status === "loaded" ? routeState : null,
  );
  const currentRouteState = resolveCurrentRouteState(
    routeState,
    routeKey,
    routeInitialData,
    taskId,
    sessionId,
  );
  useTaskDetailRouteFetch({
    taskId,
    sessionId,
    selectedSessionId: selectedSessionForNavigation,
    routeKey,
    routeInitialData,
    setRouteState,
    previousLoadedRouteRef,
    store,
    navigationContext,
    hydrationEpochsAtRequestStartRef,
  });
  useEffect(
    () => retainTaskNavigationRead(store, taskId, navigationContext),
    [navigationContext, store, taskId],
  );
  useTaskDetailRouteRecovery({
    taskId,
    selectedSessionId: selectedSessionForNavigation,
    routeKey,
    readState,
    setRouteState,
    previousLoadedRouteRef,
    store,
    navigationContext,
    hydrationEpochsAtRequestStartRef,
    appliedRecoveryCycleRef,
  });
  const view = deriveTaskDetailRouteView({
    currentRouteState,
    previousLoadedRoute: previousLoadedRouteRef.current,
    routeKey,
    taskId,
    routeSessionId: sessionId,
    selectedSessionId: selectedSessionForNavigation,
    projection,
  });
  useTaskDetailRouteEnrichment({
    view,
    hydratedState,
    store,
    setRouteState,
    previousLoadedRouteRef,
  });
  return { ...view, readState };
}

function useTaskDetailRouteEnrichment(args: {
  view: ReturnType<typeof deriveTaskDetailRouteView>;
  hydratedState: FetchedSessionData["initialState"] | null;
  store: ReturnType<typeof useAppStoreApi>;
  setRouteState: Dispatch<SetStateAction<TaskDetailRouteState>>;
  previousLoadedRouteRef: MutableRefObject<TaskDetailRouteState | null>;
}) {
  const { view, hydratedState, store, setRouteState, previousLoadedRouteRef } = args;
  useEffect(() => {
    const identity = view.enrichmentIdentity;
    if (
      !identity ||
      hydratedState !== view.initialState ||
      view.currentRouteStatus !== "loaded" ||
      view.displayedRouteKey !== view.routeKey ||
      !view.data
    ) {
      return;
    }

    const routeKey = view.routeKey;
    const navigationContext = view.navigationContext;
    if (!navigationContext || !isTaskNavigationCurrent(store, navigationContext)) return;
    const shellState: TaskDetailRouteState = {
      routeKey,
      status: "loaded",
      data: view.data,
      forceMergeSession: false,
      navigationContext,
      hydrationEpochsAtRequestStart: view.hydrationEpochsAtRequestStart,
    };
    previousLoadedRouteRef.current = shellState;
    setRouteState((current) =>
      current.routeKey === routeKey &&
      current.status === "loaded" &&
      isTaskNavigationCurrent(store, navigationContext)
        ? shellState
        : current,
    );
    const hydrationEpochsAtRequestStart = captureTaskSessionHydrationEpochs(
      store.getState(),
      view.data.task.id,
    );
    void fetchTaskNavigationEnrichment(identity, view.data.sessionId ?? undefined, { store })
      .then((enriched) => {
        if (!isTaskNavigationCurrent(store, navigationContext)) return;
        const loadedState: TaskDetailRouteState = {
          ...shellState,
          data: enriched,
          hydrationEpochsAtRequestStart,
        };
        const previousLoadedRoute = previousLoadedRouteRef.current;
        if (
          previousLoadedRoute?.status === "loaded" &&
          previousLoadedRoute.routeKey === routeKey &&
          previousLoadedRoute.navigationContext === navigationContext
        ) {
          previousLoadedRouteRef.current = loadedState;
        }
        setRouteState((current) =>
          current.routeKey === routeKey &&
          current.status === "loaded" &&
          current.navigationContext === navigationContext &&
          isTaskNavigationCurrent(store, navigationContext)
            ? loadedState
            : current,
        );
      })
      .catch((error) => {
        console.warn(
          "Could not load optional /t/:taskId route enrichment:",
          error instanceof Error ? error.message : String(error),
        );
      });
  }, [
    hydratedState,
    previousLoadedRouteRef,
    setRouteState,
    store,
    view.data,
    view.displayedRouteKey,
    view.enrichmentIdentity,
    view.hydrationEpochsAtRequestStart,
    view.navigationContext,
    view.routeKey,
    view.currentRouteStatus,
  ]);
}

function resolveCurrentRouteState(
  routeState: TaskDetailRouteState,
  routeKey: string,
  routeInitialData: FetchedSessionData | undefined,
  taskId: string,
  sessionId?: string,
): TaskDetailRouteState {
  if (routeState.routeKey === routeKey) return routeState;
  return initialRouteState(routeInitialData, taskId, sessionId);
}

function useTaskDetailRouteFetch(args: {
  taskId: string;
  sessionId?: string;
  selectedSessionId?: string;
  routeKey: string;
  routeInitialData: FetchedSessionData | undefined;
  setRouteState: Dispatch<SetStateAction<TaskDetailRouteState>>;
  previousLoadedRouteRef: MutableRefObject<TaskDetailRouteState | null>;
  store: ReturnType<typeof useAppStoreApi>;
  navigationContext: TaskNavigationContext;
  hydrationEpochsAtRequestStartRef: MutableRefObject<
    Readonly<Record<string, TaskSessionHydrationEpoch>> | undefined
  >;
}) {
  const {
    taskId,
    sessionId,
    selectedSessionId,
    routeKey,
    routeInitialData,
    setRouteState,
    previousLoadedRouteRef,
    store,
    navigationContext,
    hydrationEpochsAtRequestStartRef,
  } = args;
  useEffect(() => {
    const previous = previousLoadedRouteRef.current;
    const activeWorkspaceId = store.getState().workspaces.activeId;
    if (
      activeWorkspaceId &&
      previous?.status === "loaded" &&
      previous.routeKey === routeKey &&
      previous.data.task.workspace_id !== activeWorkspaceId
    ) {
      return;
    }
    if (routeDataMatchesSelection(routeInitialData, taskId, sessionId)) {
      const loadedState: TaskDetailRouteState = {
        routeKey,
        status: "loaded",
        data: routeInitialData,
        forceMergeSession: true,
      };
      previousLoadedRouteRef.current = loadedState;
      setRouteState(loadedState);
      return;
    }
    let cancelled = false;
    setRouteState({ routeKey, status: "loading", data: null });
    hydrationEpochsAtRequestStartRef.current = captureTaskSessionHydrationEpochs(
      store.getState(),
      taskId,
    );
    readTaskNavigationIdentity(store, taskId, { context: navigationContext })
      .then((identity) => {
        if (cancelled || !isTaskNavigationCurrent(store, navigationContext)) return;
        const shellState: TaskDetailRouteState = {
          routeKey,
          status: "loaded",
          data: buildTaskNavigationShellData(identity, selectedSessionId),
          forceMergeSession: false,
          navigationContext,
          hydrationEpochsAtRequestStart: hydrationEpochsAtRequestStartRef.current,
          enrichmentIdentity: identity,
        };
        previousLoadedRouteRef.current = shellState;
        setRouteState((current) =>
          isTaskNavigationCurrent(store, navigationContext) ? shellState : current,
        );
      })
      .catch((error) => {
        if (!cancelled && isTaskNavigationCurrent(store, navigationContext)) {
          if (!(error instanceof Error && error.name === "AbortError")) {
            console.warn(
              "Could not load /t/:taskId route data; task page will fall back to client fetches:",
              error instanceof Error ? error.message : String(error),
            );
            setRouteState({ routeKey, status: "error", data: null });
          }
        }
      });
    return () => {
      cancelled = true;
    };
  }, [
    routeInitialData,
    routeKey,
    sessionId,
    selectedSessionId,
    taskId,
    previousLoadedRouteRef,
    setRouteState,
    store,
    navigationContext,
  ]);
}

function TaskRouteLoading({
  overlay = false,
  retrying = false,
}: {
  overlay?: boolean;
  retrying?: boolean;
}) {
  const { t } = useTranslation();
  const className = overlay
    ? "absolute inset-0 z-50 flex items-center justify-center bg-background"
    : "flex h-full min-h-0 w-full items-center justify-center bg-background";
  return (
    <div className={className}>
      <p role="status" aria-live="polite" className="text-sm text-muted-foreground">
        {t(retrying ? "task:retrying" : "common:loadingTask")}
      </p>
    </div>
  );
}
