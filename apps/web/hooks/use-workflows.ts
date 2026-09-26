import { useCallback, useEffect, useRef } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { listWorkflows } from "@/lib/api";
import type { WorkflowsState } from "@/lib/state/slices";
import {
  classifyWorkspaceContextReadError,
  isCurrentWorkspaceContext,
  retryAfterMilliseconds,
} from "@/lib/state/workspace-context";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import { useForegroundRefresh } from "./use-foreground-refresh";
import { generateUUID } from "@/lib/utils";
import type { WorkspaceContextReadState } from "@/lib/state/slices/kanban/types";

const WORKSPACE_CONTEXT_RETRY_DELAYS_MS = [2_000, 5_000] as const;
const NOOP_REFRESH = () => {};

type StoreWorkflow = WorkflowsState["items"][number];
type SetWorkflows = (workflows: StoreWorkflow[]) => void;

function canRetryWorkspaceContext(
  readState: WorkspaceContextReadState | undefined,
  workspaceId: string | null,
): readState is WorkspaceContextReadState {
  return (
    document.visibilityState === "visible" &&
    workspaceId !== null &&
    readState !== undefined &&
    readState.workspaceId === workspaceId
  );
}

function hasPendingWorkspaceContextRead(readState: WorkspaceContextReadState): boolean {
  const requestIds = readState.requestIds;
  const pendingCollection = Object.entries(readState.pending).some(
    ([collection, pending]) =>
      pending &&
      (requestIds === undefined || requestIds[collection as keyof typeof requestIds] !== null),
  );
  const pendingSnapshot =
    readState.snapshotPending &&
    (readState.snapshotRequestId === undefined || readState.snapshotRequestId !== null);
  return pendingCollection || pendingSnapshot;
}

function hasTransientWorkspaceContextError(readState: WorkspaceContextReadState): boolean {
  return (
    Object.values(readState.errors).some((error) => error === "transient") ||
    readState.snapshotError === "transient"
  );
}

function workspaceContextRetryAfter(readState: WorkspaceContextReadState): number {
  return Object.entries(readState.errors).reduce(
    (maximum, [collection, error]) => {
      if (error !== "transient") return maximum;
      return Math.max(
        maximum,
        readState.retryAfterMs[collection as keyof typeof readState.retryAfterMs] ?? 0,
      );
    },
    readState.snapshotError === "transient" ? (readState.snapshotRetryAfterMs ?? 0) : 0,
  );
}

/**
 * Fire-and-forget fetch effect. Kept internal so callers that only need to
 * populate `state.workflows.items` (e.g. `useEnsureWorkspaceWorkflows`) don't
 * also subscribe to the store slice they wrote to — that would re-render the
 * caller on every fetch and defeats the "top-level layout" placement.
 */
// eslint-disable-next-line max-params, max-lines-per-function -- the effect is shared by active and passive workflow consumers
function useWorkflowsFetchEffect(
  workspaceId: string | null,
  enabled: boolean,
  requireActiveWorkspace: boolean,
  setWorkflows: SetWorkflows,
  store: StoreApi<AppState>,
  trackRecovery: boolean,
  retryVersion: number,
) {
  useEffect(() => {
    if (!enabled || !workspaceId) return;
    let cancelled = false;
    const requestId = trackRecovery ? generateUUID() : undefined;
    const generation = store.getState().workspaceContextGeneration;
    if (trackRecovery && typeof store.getState().setWorkspaceContextRead === "function") {
      store
        .getState()
        .setWorkspaceContextRead(
          "workflows",
          workspaceId,
          generation,
          "pending",
          undefined,
          requestId,
        );
    }
    listWorkflows(workspaceId, { cache: "no-store", includeHidden: true })
      .then((response) => {
        const state = store.getState();
        const staleWorkspaceContext = requireActiveWorkspace
          ? !isCurrentWorkspaceContext(state, workspaceId, generation)
          : state.workspaceContextGeneration !== generation;
        if (cancelled || staleWorkspaceContext) {
          return;
        }
        const mapped = response.workflows.map((workflow) => ({
          id: workflow.id,
          workspaceId: workflow.workspace_id,
          name: workflow.name,
          description: workflow.description,
          prompt: workflow.prompt,
          sortOrder: workflow.sort_order ?? 0,
          agent_profile_id: workflow.agent_profile_id,
          hidden: workflow.hidden,
          style: workflow.style,
        }));
        setWorkflows(mapped);
        if (trackRecovery && typeof state.setWorkspaceContextRead === "function") {
          state.setWorkspaceContextRead(
            "workflows",
            workspaceId,
            generation,
            "success",
            undefined,
            requestId,
          );
        }
      })
      // Do not clear on error — the sidebar mounts on every route, and boot
      // hydrates workflows before the refresh fires. Blowing the slice away on
      // a network flake would leave the sidebar and board with no workflow IDs
      // until another success. The next successful fetch replaces the slice.
      .catch((error: unknown) => {
        const state = store.getState();
        const staleWorkspaceContext = requireActiveWorkspace
          ? !isCurrentWorkspaceContext(state, workspaceId, generation)
          : state.workspaceContextGeneration !== generation;
        if (
          cancelled ||
          staleWorkspaceContext ||
          !trackRecovery ||
          typeof state.setWorkspaceContextRead !== "function"
        )
          return;
        state.setWorkspaceContextRead(
          "workflows",
          workspaceId,
          generation,
          classifyWorkspaceContextReadError(error),
          retryAfterMilliseconds(error),
          requestId,
        );
      });
    return () => {
      cancelled = true;
      if (
        trackRecovery &&
        requestId &&
        typeof store.getState().setWorkspaceContextRead === "function"
      ) {
        store
          .getState()
          .setWorkspaceContextRead(
            "workflows",
            workspaceId,
            generation,
            "cancelled",
            undefined,
            requestId,
          );
      }
    };
  }, [
    enabled,
    requireActiveWorkspace,
    retryVersion,
    setWorkflows,
    store,
    trackRecovery,
    workspaceId,
  ]);
}

/**
 * Load workflows for the active workspace. Call from a component that stays
 * mounted independently of any collapsible section, so `state.workflows.items`
 * follows the active workspace even when the sidebar's Tasks section is
 * collapsed and its children (which consume workflows) are unmounted.
 */
export function useEnsureWorkspaceWorkflows() {
  const store = useAppStoreApi();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const setWorkflows = useAppStore((state) => state.setWorkflows);
  const readState = useAppStore((state) => state.workspaceContextRead);
  const retryVersion = readState?.retryVersion ?? 0;
  const requestRefresh = useAppStore(
    (state) => state.requestWorkspaceContextRefresh ?? NOOP_REFRESH,
  );
  const retryCycleRef = useRef({ key: "", attempts: 0 });
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const retryTimerKeyRef = useRef<string | null>(null);
  const retryTimerTokenRef = useRef(0);

  useWorkflowsFetchEffect(workspaceId, true, true, setWorkflows, store, true, retryVersion);

  const recoverOnForeground = useCallback(() => {
    if (!workspaceId || !readState || readState.workspaceId !== workspaceId) return;
    if (
      hasPendingWorkspaceContextRead(readState) ||
      (!Object.values(readState.errors).some((error) => error === "transient") &&
        readState.snapshotError !== "transient")
    )
      return;
    requestRefresh();
  }, [readState, requestRefresh, workspaceId]);

  useForegroundRefresh(recoverOnForeground, Boolean(workspaceId), workspaceId);

  useEffect(() => {
    const clearRetryTimer = () => {
      if (retryTimerRef.current) clearTimeout(retryTimerRef.current);
      retryTimerRef.current = null;
      retryTimerKeyRef.current = null;
    };

    if (!canRetryWorkspaceContext(readState, workspaceId)) {
      clearRetryTimer();
      return;
    }
    if (
      hasPendingWorkspaceContextRead(readState) ||
      !hasTransientWorkspaceContextError(readState)
    ) {
      clearRetryTimer();
      return;
    }

    const key = `${workspaceId}:${readState.generation}:${readState.retryCycle}`;
    if (retryCycleRef.current.key !== key) {
      clearRetryTimer();
      retryCycleRef.current = { key, attempts: 0 };
    }
    if (retryCycleRef.current.attempts >= WORKSPACE_CONTEXT_RETRY_DELAYS_MS.length) return;
    if (retryTimerRef.current && retryTimerKeyRef.current === key) return;
    if (retryTimerRef.current) clearRetryTimer();

    const retryIndex = retryCycleRef.current.attempts;
    const delay = Math.max(
      WORKSPACE_CONTEXT_RETRY_DELAYS_MS[retryIndex],
      workspaceContextRetryAfter(readState),
    );
    retryTimerKeyRef.current = key;
    const timerToken = ++retryTimerTokenRef.current;
    retryTimerRef.current = setTimeout(() => {
      if (retryTimerKeyRef.current !== key || retryTimerTokenRef.current !== timerToken) return;
      retryTimerRef.current = null;
      retryTimerKeyRef.current = null;
      retryTimerTokenRef.current += 1;
      if (document.visibilityState !== "visible") return;
      const currentReadState = store.getState().workspaceContextRead;
      if (
        !canRetryWorkspaceContext(currentReadState, workspaceId) ||
        hasPendingWorkspaceContextRead(currentReadState) ||
        !hasTransientWorkspaceContextError(currentReadState)
      )
        return;
      retryCycleRef.current.attempts += 1;
      requestRefresh(false);
    }, delay);
  }, [readState, requestRefresh, retryVersion, store, workspaceId]);

  useEffect(() => {
    const clearWhenHidden = () => {
      if (document.visibilityState !== "hidden") return;
      if (retryTimerRef.current) {
        clearTimeout(retryTimerRef.current);
        retryTimerTokenRef.current += 1;
      }
      retryTimerRef.current = null;
      retryTimerKeyRef.current = null;
    };
    document.addEventListener("visibilitychange", clearWhenHidden);
    return () => {
      document.removeEventListener("visibilitychange", clearWhenHidden);
      if (retryTimerRef.current) clearTimeout(retryTimerRef.current);
      retryTimerRef.current = null;
      retryTimerKeyRef.current = null;
      retryTimerTokenRef.current += 1;
    };
  }, []);
}

export function useWorkflows(workspaceId: string | null, enabled = true) {
  const store = useAppStoreApi();
  const workflows = useAppStore((state) => state.workflows.items);
  const setWorkflows = useAppStore((state) => state.setWorkflows);
  useWorkflowsFetchEffect(workspaceId, enabled, false, setWorkflows, store, false, 0);
  return { workflows };
}
