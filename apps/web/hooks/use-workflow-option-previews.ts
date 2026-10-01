"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { listWorkflowSteps } from "@/lib/api/domains/workflow-api";

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
  workflowIds: Set<string>;
  active: boolean;
  pending: Set<string>;
  requestIds: Map<string, number>;
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

  void listWorkflowSteps(workflowId, { cache: "no-store" })
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
  const workflowIdsKey = JSON.stringify([...new Set(workflowIds)].sort());
  const normalizedWorkflowIds = JSON.parse(workflowIdsKey) as string[];
  const scopeIdentity =
    open && workspaceId ? JSON.stringify([workspaceId, workflowIdsKey, refreshKey]) : null;
  const [generation, setGeneration] = useState({ scopeIdentity, value: 0 });
  const currentGeneration =
    generation.scopeIdentity === scopeIdentity ? generation.value : generation.value + 1;
  if (generation.scopeIdentity !== scopeIdentity) {
    setGeneration({ scopeIdentity, value: currentGeneration });
  }
  const scopeKey = scopeIdentity ? `${scopeIdentity}:${currentGeneration}` : null;
  const requestSequence = useRef(0);
  const cycleRef = useRef<RequestCycle | null>(null);
  const [state, setState] = useState<PreviewState>({ scopeKey: null, entries: {} });

  if (state.scopeKey !== scopeKey) {
    setState({ scopeKey, entries: scopeKey ? loadingEntries(normalizedWorkflowIds) : {} });
  }

  useEffect(() => {
    if (!scopeKey || !workspaceId) return;
    const cycle: RequestCycle = {
      scopeKey,
      workflowIds: new Set(JSON.parse(workflowIdsKey) as string[]),
      active: true,
      pending: new Set(),
      requestIds: new Map(),
    };
    cycleRef.current = cycle;
    for (const workflowId of cycle.workflowIds) {
      requestPreview(cycle, workflowId, requestSequence, setState);
    }
    return () => {
      cycle.active = false;
      if (cycleRef.current === cycle) cycleRef.current = null;
    };
    // workflowIdsKey provides stable membership while callers rebuild arrays.
  }, [scopeKey, workspaceId, workflowIdsKey]);

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
      requestPreview(cycle, workflowId, requestSequence, setState);
    },
    [scopeKey, state],
  );

  return { previews, retry };
}
