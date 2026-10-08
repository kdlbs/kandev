import { useEffect, type Dispatch, type MutableRefObject, type SetStateAction } from "react";
import type { StoreApi } from "zustand";
import { buildTaskNavigationShellData } from "@/lib/ssr/session-page-state";
import { captureTaskSessionHydrationEpochs } from "@/lib/state/slices/session/hydration-epochs";
import type { TaskSessionHydrationEpoch } from "@/lib/state/slices/session/types";
import type { AppState } from "@/lib/state/store";
import {
  isTaskNavigationCurrent,
  type TaskNavigationContext,
  type TaskNavigationReadSnapshot,
} from "@/lib/state/task-navigation-reads";
import type { TaskDetailRouteState } from "./task-detail-route-state";

type TaskDetailRouteRecoveryArgs = {
  taskId: string;
  selectedSessionId?: string;
  routeKey: string;
  readState: TaskNavigationReadSnapshot;
  setRouteState: Dispatch<SetStateAction<TaskDetailRouteState>>;
  previousLoadedRouteRef: MutableRefObject<TaskDetailRouteState | null>;
  store: StoreApi<AppState>;
  navigationContext: TaskNavigationContext;
  hydrationEpochsAtRequestStartRef: MutableRefObject<
    Readonly<Record<string, TaskSessionHydrationEpoch>> | undefined
  >;
  appliedRecoveryCycleRef: MutableRefObject<{ generation: number; cycle: number }>;
};

export function useTaskDetailRouteRecovery(args: TaskDetailRouteRecoveryArgs) {
  const {
    taskId,
    selectedSessionId,
    routeKey,
    readState,
    setRouteState,
    previousLoadedRouteRef,
    store,
    navigationContext,
    hydrationEpochsAtRequestStartRef,
    appliedRecoveryCycleRef,
  } = args;
  useEffect(() => {
    if (
      readState.phase !== "succeeded" ||
      readState.cycle < 2 ||
      readState.cycle <= appliedRecoveryCycleRef.current.cycle ||
      !readState.identity ||
      !isTaskNavigationCurrent(store, navigationContext)
    ) {
      return;
    }
    const alreadyLoadedSameRoute =
      previousLoadedRouteRef.current?.status === "loaded" &&
      previousLoadedRouteRef.current.routeKey === routeKey;
    if (alreadyLoadedSameRoute) {
      appliedRecoveryCycleRef.current.cycle = readState.cycle;
      return;
    }
    const state = store.getState();
    const loadedState: TaskDetailRouteState = {
      routeKey,
      status: "loaded",
      data: buildTaskNavigationShellData(readState.identity, selectedSessionId),
      forceMergeSession: false,
      navigationContext,
      hydrationEpochsAtRequestStart:
        hydrationEpochsAtRequestStartRef.current ??
        captureTaskSessionHydrationEpochs(state, taskId),
      enrichmentIdentity: readState.identity,
    };
    appliedRecoveryCycleRef.current.cycle = readState.cycle;
    previousLoadedRouteRef.current = loadedState;
    setRouteState((current) => (current.routeKey === routeKey ? loadedState : current));
  }, [
    appliedRecoveryCycleRef,
    navigationContext,
    previousLoadedRouteRef,
    readState.identity,
    readState.phase,
    readState.revision,
    routeKey,
    selectedSessionId,
    setRouteState,
    store,
    taskId,
    hydrationEpochsAtRequestStartRef,
  ]);
}
