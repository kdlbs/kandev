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
  readTaskNavigationIdentity,
} from "@/lib/state/task-navigation-reads";
import type {
  TaskNavigationContext,
  TaskNavigationIdentity,
} from "@/lib/state/task-navigation-reads";
import type { Task } from "@/lib/types/http";
import { getOwnedTaskSessionId, useTaskRouteProjection } from "./task-route-projection";

type TaskDetailRouteProps = {
  taskId: string;
  sessionId?: string;
  layout?: string | null;
  simple?: string;
  mode?: string;
  initialData?: FetchedSessionData;
};

type TaskDetailRouteState =
  | { routeKey: string; status: "loading"; data: null }
  | {
      routeKey: string;
      status: "loaded";
      data: FetchedSessionData;
      forceMergeSession: boolean;
      navigationContext?: TaskNavigationContext;
      hydrationEpochsAtRequestStart?: Readonly<Record<string, TaskSessionHydrationEpoch>>;
      enrichmentIdentity?: TaskNavigationIdentity;
    }
  | { routeKey: string; status: "error"; data: null };

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
    return <TaskRouteLoading />;
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

type TaskDetailRouteView = ReturnType<typeof deriveTaskDetailRouteView>;

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
      {route.isLoadingOverPreviousRoute ? <TaskRouteLoading overlay /> : null}
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
    routeKey,
    routeInitialData,
    setRouteState,
    previousLoadedRouteRef,
    store,
    navigationContext,
  });
  const view = deriveTaskDetailRouteView(
    currentRouteState,
    previousLoadedRouteRef.current,
    taskId,
    sessionId,
    projection,
  );
  useTaskDetailRouteEnrichment({
    view,
    hydratedState,
    store,
    setRouteState,
    previousLoadedRouteRef,
  });
  return view;
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
  routeKey: string;
  routeInitialData: FetchedSessionData | undefined;
  setRouteState: Dispatch<SetStateAction<TaskDetailRouteState>>;
  previousLoadedRouteRef: MutableRefObject<TaskDetailRouteState | null>;
  store: ReturnType<typeof useAppStoreApi>;
  navigationContext: TaskNavigationContext;
}) {
  const {
    taskId,
    sessionId,
    routeKey,
    routeInitialData,
    setRouteState,
    previousLoadedRouteRef,
    store,
    navigationContext,
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
    const hydrationEpochsAtRequestStart = captureTaskSessionHydrationEpochs(
      store.getState(),
      taskId,
    );
    const requestState = store.getState();
    const selectedSessionId =
      sessionId ??
      getOwnedTaskSessionId(requestState, taskId, requestState.tasks.activeSessionId) ??
      undefined;
    readTaskNavigationIdentity(store, taskId, { context: navigationContext })
      .then((identity) => {
        if (cancelled || !isTaskNavigationCurrent(store, navigationContext)) return;
        const shellState: TaskDetailRouteState = {
          routeKey,
          status: "loaded",
          data: buildTaskNavigationShellData(identity, selectedSessionId),
          forceMergeSession: false,
          navigationContext,
          hydrationEpochsAtRequestStart,
          enrichmentIdentity: identity,
        };
        previousLoadedRouteRef.current = shellState;
        setRouteState((current) =>
          isTaskNavigationCurrent(store, navigationContext) ? shellState : current,
        );
      })
      .catch((error) => {
        if (!cancelled && isTaskNavigationCurrent(store, navigationContext)) {
          console.warn(
            "Could not load /t/:taskId route data; task page will fall back to client fetches:",
            error instanceof Error ? error.message : String(error),
          );
          setRouteState({ routeKey, status: "error", data: null });
        }
      });
    return () => {
      cancelled = true;
    };
  }, [
    routeInitialData,
    routeKey,
    sessionId,
    taskId,
    previousLoadedRouteRef,
    setRouteState,
    store,
    navigationContext,
  ]);
}

function deriveTaskDetailRouteView(
  currentRouteState: TaskDetailRouteState,
  previousLoadedRoute: TaskDetailRouteState | null,
  taskId: string,
  sessionId?: string,
  projection?: ReturnType<typeof useTaskRouteProjection>,
) {
  const view = deriveFallbackTaskDetailRouteView(
    currentRouteState,
    previousLoadedRoute,
    taskId,
    sessionId,
  );
  if (currentRouteState.status !== "loading" || !projection) return view;
  return {
    ...view,
    displayedRouteKey: null,
    data: null,
    task: projection.task,
    initialState: null,
    activeSessionId: projection.sessionId,
    forceMergeSession: false,
    hydrationEpochsAtRequestStart: undefined,
    shellTaskId: taskId,
    isLoadingOverPreviousRoute: false,
    showShell: true,
    showInitialLoading: false,
  };
}

function deriveFallbackTaskDetailRouteView(
  currentRouteState: TaskDetailRouteState,
  previousLoadedRoute: TaskDetailRouteState | null,
  taskId: string,
  sessionId?: string,
) {
  const displayedRouteState = resolveDisplayedRouteState(currentRouteState, previousLoadedRoute);
  const isLoadingOverPreviousRoute = isLoadingWithDisplayedRoute(
    currentRouteState,
    displayedRouteState,
  );
  const data = routeDataFromState(displayedRouteState);
  const activeSessionId = routeSessionFromState(displayedRouteState, sessionId);
  const forceMergeSession = shouldForceMergeRouteState(displayedRouteState);
  const initialState = data?.initialState ?? null;
  const task = data?.task ?? null;
  const shellTaskId = resolveShellTaskId(isLoadingOverPreviousRoute, task, taskId);
  const routeMetadata = loadedRouteMetadata(displayedRouteState);

  return {
    routeKey: taskRouteKey(taskId, sessionId),
    currentRouteStatus: currentRouteState.status,
    displayedRouteKey: displayedRouteState?.routeKey ?? null,
    data,
    task,
    initialState,
    activeSessionId,
    forceMergeSession,
    ...routeMetadata,
    shellTaskId,
    isLoadingOverPreviousRoute,
    showShell: displayedRouteState !== null || currentRouteState.status !== "loading",
    showInitialLoading: currentRouteState.status === "loading" && displayedRouteState === null,
  };
}

function isLoadingWithDisplayedRoute(
  currentRouteState: TaskDetailRouteState,
  displayedRouteState: TaskDetailRouteState | null,
): boolean {
  return currentRouteState.status === "loading" && displayedRouteState !== null;
}

function resolveShellTaskId(
  isLoadingOverPreviousRoute: boolean,
  task: Task | null,
  taskId: string,
): string {
  if (!isLoadingOverPreviousRoute) return taskId;
  return task?.id ?? taskId;
}

function loadedRouteMetadata(state: TaskDetailRouteState | null) {
  if (state?.status !== "loaded") {
    return {
      navigationContext: undefined,
      enrichmentIdentity: undefined,
      hydrationEpochsAtRequestStart: undefined,
    };
  }
  return {
    navigationContext: state.navigationContext,
    enrichmentIdentity: state.enrichmentIdentity,
    hydrationEpochsAtRequestStart: state.hydrationEpochsAtRequestStart,
  };
}

function resolveDisplayedRouteState(
  currentRouteState: TaskDetailRouteState,
  previousLoadedRoute: TaskDetailRouteState | null,
): TaskDetailRouteState | null {
  if (currentRouteState.status === "loaded") return currentRouteState;
  if (currentRouteState.status === "loading") return previousLoadedRoute;
  return null;
}

function routeDataFromState(state: TaskDetailRouteState | null): FetchedSessionData | null {
  if (state?.status === "loaded") return state.data;
  return null;
}

function routeSessionFromState(
  state: TaskDetailRouteState | null,
  fallbackSessionId?: string,
): string | null {
  if (state?.status === "loaded") return state.data.sessionId ?? null;
  return fallbackSessionId ?? null;
}

function shouldForceMergeRouteState(state: TaskDetailRouteState | null): boolean {
  return state?.status === "loaded" && state.forceMergeSession;
}

function TaskRouteLoading({ overlay = false }: { overlay?: boolean }) {
  const { t } = useTranslation();
  const className = overlay
    ? "absolute inset-0 z-50 flex items-center justify-center bg-background"
    : "flex h-full min-h-0 w-full items-center justify-center bg-background";
  return (
    <div className={className}>
      <p role="status" aria-live="polite" className="text-sm text-muted-foreground">
        {t("common:loadingTask")}
      </p>
    </div>
  );
}
