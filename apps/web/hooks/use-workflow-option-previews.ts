"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { readJourneyWorkflowSteps } from "@/hooks/journey-metadata-resources";
import { useOptionalAppStore, useOptionalAppStoreApi } from "@/components/state-provider";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";

export type WorkflowOptionPreviewStep = {
  id: string;
  title: string;
  color: string;
  position: number;
  agent_profile_id?: string;
  is_start_step?: boolean;
};

export type WorkflowOptionPreview =
  | { status: "loading" }
  | { status: "success"; steps: WorkflowOptionPreviewStep[] }
  | { status: "error" };

type StoredPreview =
  | { status: "loading"; requestId: number }
  | { status: "success"; steps: WorkflowOptionPreviewStep[] }
  | { status: "error" };
type PreviewState = { scopeKey: string | null; entries: Record<string, StoredPreview> };
type RequestCycle = {
  scopeKey: string;
  refresh: boolean;
  workflowIds: Set<string>;
  active: boolean;
  pending: Set<string>;
  requestIds: Map<string, number>;
  controller: AbortController;
};

function loadingEntries(workflowIds: string[]): Record<string, StoredPreview> {
  return Object.fromEntries(
    workflowIds.map((workflowId) => [workflowId, { status: "loading", requestId: 0 }]),
  );
}

function updatePreview(
  setState: Dispatch<SetStateAction<PreviewState>>,
  cycle: RequestCycle,
  workflowId: string,
  requestId: number,
  preview: StoredPreview,
) {
  if (!cycle.active || cycle.requestIds.get(workflowId) !== requestId) return;
  setState((current) => {
    if (current.scopeKey !== cycle.scopeKey) return current;
    if (current.entries[workflowId]?.status === undefined) return current;
    if (
      current.entries[workflowId]?.status === "loading" &&
      current.entries[workflowId]?.requestId !== requestId
    ) {
      return current;
    }
    return { ...current, entries: { ...current.entries, [workflowId]: preview } };
  });
}

function requestPreview(
  cycle: RequestCycle,
  workflowId: string,
  requestSequence: { current: number },
  setState: Dispatch<SetStateAction<PreviewState>>,
  store: StoreApi<AppState> | undefined,
) {
  if (!cycle.active || cycle.pending.has(workflowId)) return;
  const requestId = ++requestSequence.current;
  cycle.pending.add(workflowId);
  cycle.requestIds.set(workflowId, requestId);
  setState((current) => {
    if (current.scopeKey !== cycle.scopeKey || !current.entries[workflowId]) return current;
    return {
      ...current,
      entries: {
        ...current.entries,
        [workflowId]: { status: "loading", requestId },
      },
    };
  });

  void readJourneyWorkflowSteps(store, workflowId, {
    signal: cycle.controller.signal,
    refresh: cycle.refresh,
  })
    .then((response) => {
      const steps = [...response.steps]
        .sort((left, right) => left.position - right.position)
        .map((step) => ({
          id: step.id,
          title: step.name,
          color: step.color,
          position: step.position,
          is_start_step: step.is_start_step,
          agent_profile_id: step.agent_profile_id,
        }));
      cycle.pending.delete(workflowId);
      updatePreview(setState, cycle, workflowId, requestId, { status: "success", steps });
    })
    .catch(() => {
      cycle.pending.delete(workflowId);
      updatePreview(setState, cycle, workflowId, requestId, { status: "error" });
    });
}

export function useWorkflowOptionPreviews(
  workspaceId: string | null | undefined,
  open: boolean,
  workflowIds: string[],
  refreshKey: number | string = 0,
): {
  previews: Record<string, WorkflowOptionPreview>;
  retry: (workflowId: string) => void;
} {
  const store = useOptionalAppStoreApi() ?? undefined;
  const readScope = useOptionalAppStore(stateReadScopeIdentity, "");
  const workflowIdsKey = JSON.stringify([...new Set(workflowIds)].sort());
  const normalizedWorkflowIds = JSON.parse(workflowIdsKey) as string[];
  const scopeIdentity =
    open && workspaceId
      ? JSON.stringify([workspaceId, workflowIdsKey, refreshKey, readScope])
      : null;
  const [generation, setGeneration] = useState({ scopeIdentity, value: 0 });
  const currentGeneration =
    generation.scopeIdentity === scopeIdentity ? generation.value : generation.value + 1;
  if (generation.scopeIdentity !== scopeIdentity) {
    setGeneration({ scopeIdentity, value: currentGeneration });
  }
  const scopeKey = scopeIdentity ? `${scopeIdentity}:${currentGeneration}` : null;
  const requestSequence = useRef(0);
  const cycleRef = useRef<RequestCycle | null>(null);
  const previousRefresh = useRef({ workspaceId, readScope, refreshKey });
  const [state, setState] = useState<PreviewState>({ scopeKey: null, entries: {} });

  if (state.scopeKey !== scopeKey) {
    setState({ scopeKey, entries: scopeKey ? loadingEntries(normalizedWorkflowIds) : {} });
  }

  useEffect(() => {
    if (!scopeKey || !workspaceId) return;
    const previous = previousRefresh.current;
    const refresh =
      previous.workspaceId === workspaceId &&
      previous.readScope === readScope &&
      previous.refreshKey !== refreshKey;
    previousRefresh.current = { workspaceId, readScope, refreshKey };
    const cycle: RequestCycle = {
      scopeKey,
      refresh,
      workflowIds: new Set(JSON.parse(workflowIdsKey) as string[]),
      active: true,
      pending: new Set(),
      requestIds: new Map(),
      controller: new AbortController(),
    };
    cycleRef.current = cycle;
    for (const workflowId of cycle.workflowIds) {
      requestPreview(cycle, workflowId, requestSequence, setState, store);
    }
    return () => {
      cycle.active = false;
      cycle.controller.abort();
      if (cycleRef.current === cycle) cycleRef.current = null;
    };
    // workflowIdsKey provides stable membership while callers rebuild arrays.
  }, [scopeKey, workspaceId, workflowIdsKey, store, readScope, refreshKey]);

  const previews: Record<string, WorkflowOptionPreview> = {};
  if (scopeKey) {
    const current =
      state.scopeKey === scopeKey ? state.entries : loadingEntries(normalizedWorkflowIds);
    for (const [workflowId, preview] of Object.entries(current)) {
      if (preview.status === "loading") {
        previews[workflowId] = { status: "loading" };
      } else {
        previews[workflowId] = preview;
      }
    }
  }

  const retry = useCallback(
    (workflowId: string) => {
      const cycle = cycleRef.current;
      if (
        !scopeKey ||
        !cycle?.active ||
        cycle.scopeKey !== scopeKey ||
        !cycle.workflowIds.has(workflowId) ||
        cycle.pending.has(workflowId) ||
        state.scopeKey !== scopeKey ||
        state.entries[workflowId]?.status !== "error"
      ) {
        return;
      }
      requestPreview(cycle, workflowId, requestSequence, setState, store);
    },
    [scopeKey, state, store],
  );

  return { previews, retry };
}
