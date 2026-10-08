import type { FetchedSessionData } from "@/lib/ssr/session-page-state";
import type { Task } from "@/lib/types/http";
import type { TaskDetailRouteState } from "./task-detail-route-state";
import type { useTaskRouteProjection } from "./task-route-projection";

type DeriveTaskDetailRouteViewArgs = {
  currentRouteState: TaskDetailRouteState;
  previousLoadedRoute: TaskDetailRouteState | null;
  routeKey: string;
  taskId: string;
  routeSessionId?: string;
  selectedSessionId?: string;
  projection?: ReturnType<typeof useTaskRouteProjection>;
};

export function deriveTaskDetailRouteView({
  currentRouteState,
  previousLoadedRoute,
  routeKey,
  taskId,
  routeSessionId,
  selectedSessionId,
  projection,
}: DeriveTaskDetailRouteViewArgs) {
  const view = deriveFallbackTaskDetailRouteView({
    currentRouteState,
    previousLoadedRoute,
    routeKey,
    taskId,
    routeSessionId,
    selectedSessionId,
  });
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

type FallbackViewArgs = Omit<DeriveTaskDetailRouteViewArgs, "projection">;

function deriveFallbackTaskDetailRouteView({
  currentRouteState,
  previousLoadedRoute,
  routeKey,
  taskId,
  routeSessionId,
  selectedSessionId,
}: FallbackViewArgs) {
  const displayedRouteState = resolveDisplayedRouteState(currentRouteState, previousLoadedRoute);
  const isLoadingOverPreviousRoute = isLoadingWithDisplayedRoute(
    currentRouteState,
    displayedRouteState,
  );
  const data = routeDataFromState(displayedRouteState);
  const activeSessionId = routeSessionFromState(
    displayedRouteState,
    selectedSessionId ?? routeSessionId,
  );
  const forceMergeSession = shouldForceMergeRouteState(displayedRouteState);
  const initialState = data?.initialState ?? null;
  const task = data?.task ?? null;
  const shellTaskId = resolveShellTaskId(isLoadingOverPreviousRoute, task, taskId);
  const routeMetadata = loadedRouteMetadata(displayedRouteState);

  return {
    routeKey,
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
