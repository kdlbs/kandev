import { useCallback, useEffect, useMemo, useState } from "react";
import { useOptionalAppStore, useOptionalAppStoreApi } from "@/components/state-provider";
import { readJourneyWorkflowSteps } from "@/hooks/journey-metadata-resources";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import { workflowStepToState } from "@/lib/ssr/mapper";
import type {
  StepType,
  TaskCreateWorkflowStepsResult,
} from "@/components/task-create-dialog-types";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import type { WorkflowStep } from "@/lib/types/http";

type StoredWorkflowSteps = {
  identity: string;
  status: "loading" | "success" | "error";
  steps: StepType[];
};

type HookArgs = {
  open: boolean;
  /** Only task creation consumes selected metadata for previews and replacements. */
  enabled?: boolean;
  workspaceId: string | null;
  workflowId: string | null;
  openCycle: number;
  /** Complete, current-workspace board snapshot, when one exists. */
  loadedSnapshot: WorkflowSnapshotData | null;
};

function byPosition(left: { position?: number }, right: { position?: number }): number {
  return (left.position ?? Number.MAX_SAFE_INTEGER) - (right.position ?? Number.MAX_SAFE_INTEGER);
}

function fromSnapshot(workflowId: string, steps: StepType[]): StepType[] {
  return [...steps].sort(byPosition).map((step) => ({ ...step, workflowId }));
}

function fromResponse(workflowId: string, steps: WorkflowStep[]): StepType[] {
  return [...steps].sort(byPosition).map((step) => ({ ...workflowStepToState(step), workflowId }));
}

function isAbortError(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
}

function snapshotMetadata(
  workflowId: string | null,
  loadedSnapshot: WorkflowSnapshotData | null,
): StepType[] | null {
  if (!workflowId || !loadedSnapshot) return null;
  if (
    loadedSnapshot.workflowId !== workflowId ||
    loadedSnapshot.isPlaceholder ||
    loadedSnapshot.fetchFailed
  ) {
    return null;
  }
  return fromSnapshot(workflowId, loadedSnapshot.steps);
}

/**
 * Every selected-workflow activation gets a fresh identity, including a return
 * to a workflow visited earlier in the same dialog session. Without this, a
 * stored result could become authoritative again after switching through a
 * snapshot-backed workflow.
 */
function useActivationKey(scopeKey: string | null): string | null {
  const [activation, setActivation] = useState<{ scopeKey: string | null; value: number }>(() => ({
    scopeKey,
    value: 0,
  }));
  const generation = activation.scopeKey === scopeKey ? activation.value : activation.value + 1;
  if (activation.scopeKey !== scopeKey) {
    setActivation({ scopeKey, value: generation });
  }
  return scopeKey ? `${scopeKey}:${generation}` : null;
}

/**
 * Resolves complete step definitions for the workflow selected in the task
 * dialog. A complete board snapshot in the current workspace supplies the
 * result without another request; otherwise the shared lightweight step read
 * is the only demand this observer establishes.
 */
export function useTaskCreateWorkflowSteps({
  open,
  enabled = true,
  workspaceId,
  workflowId,
  openCycle,
  loadedSnapshot,
}: HookArgs): TaskCreateWorkflowStepsResult {
  const store = useOptionalAppStoreApi() ?? undefined;
  const readScope = useOptionalAppStore(stateReadScopeIdentity, "");
  // Snapshot steps are recomputed with the live snapshot so workflow-step
  // events remain visible instead of freezing the initial seed.
  const snapshotSteps = useMemo(
    () => snapshotMetadata(workflowId, loadedSnapshot),
    [loadedSnapshot, workflowId],
  );
  // The form normalizes its first open cycle from 0 to 1 in an effect. Treat
  // both as the same dialog session so that normalization cannot repeat demand.
  const dialogCycle = openCycle || 1;
  // Snapshot-backed and read-backed activations are distinct identities, so a
  // stored read result cannot reappear after a snapshot-backed visit.
  const scopeKey =
    enabled && open && workspaceId && workflowId
      ? JSON.stringify([readScope, workspaceId, workflowId, dialogCycle, snapshotSteps !== null])
      : null;
  const activationKey = useActivationKey(scopeKey);
  const [retryState, setRetryState] = useState<{ key: string | null; attempt: number }>({
    key: null,
    attempt: 0,
  });
  const retryAttempt = activationKey && retryState.key === activationKey ? retryState.attempt : 0;
  const identity = activationKey ? `${activationKey}:${retryAttempt}` : null;
  const [stored, setStored] = useState<StoredWorkflowSteps | null>(null);

  useEffect(() => {
    if (!identity || !workflowId || snapshotSteps !== null) return;
    const controller = new AbortController();
    setStored((current) =>
      current?.identity === identity ? current : { identity, status: "loading", steps: [] },
    );
    void readJourneyWorkflowSteps(store, workflowId, {
      signal: controller.signal,
      refresh: retryAttempt > 0,
    })
      .then((response) => {
        if (controller.signal.aborted) return;
        setStored({
          identity,
          status: "success",
          steps: fromResponse(workflowId, response.steps),
        });
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted || isAbortError(error)) return;
        setStored((current) =>
          current?.identity === identity
            ? { ...current, status: "error" }
            : { identity, status: "error", steps: [] },
        );
      });
    return () => controller.abort();
  }, [identity, retryAttempt, snapshotSteps, store, workflowId]);

  const retry = useCallback(() => {
    if (!activationKey) return;
    setRetryState((current) => ({
      key: activationKey,
      attempt: current.key === activationKey ? current.attempt + 1 : 1,
    }));
  }, [activationKey]);

  if (!identity) return { status: "idle", steps: [], retry };
  if (snapshotSteps !== null) return { status: "success", steps: snapshotSteps, retry };
  if (stored?.identity === identity) {
    return { status: stored.status, steps: stored.steps, retry };
  }
  return { status: "loading", steps: [], retry };
}
