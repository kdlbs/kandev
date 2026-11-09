import { useCallback, useRef, useState } from "react";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import type { TaskWorkflowCoverage } from "@/lib/types/http";

type WorkflowRef = { id: string };

export function isBootHydratedSnapshot(snapshot: WorkflowSnapshotData | undefined): boolean {
  return !!snapshot && snapshot.isPlaceholder !== true && snapshot.fetchFailed !== true;
}

export function canUseBootSnapshots(
  connectionStatus: string,
  workflows: readonly WorkflowRef[],
  snapshots: Record<string, WorkflowSnapshotData>,
): boolean {
  return (
    connectionStatus !== "connected" &&
    workflows.every((workflow) => isBootHydratedSnapshot(snapshots[workflow.id]))
  );
}

export function isEmptyWorkflowInventoryReady(
  workspaceId: string | null,
  workflowCount: number,
  coverage: TaskWorkflowCoverage | undefined,
): boolean {
  return (
    workspaceId !== null &&
    workflowCount === 0 &&
    coverage?.workspace_id === workspaceId &&
    coverage.complete
  );
}

export function useWorkflowSnapshotReadiness(workspaceId: string | null) {
  const workspaceIdentityRef = useRef({ id: workspaceId, generation: 0 });
  if (workspaceIdentityRef.current.id !== workspaceId) {
    workspaceIdentityRef.current = {
      id: workspaceId,
      generation: workspaceIdentityRef.current.generation + 1,
    };
  }
  const workspaceGeneration = workspaceIdentityRef.current.generation;
  const [initialLoadGeneration, setInitialLoadGeneration] = useState<number | null>(null);
  const markInitialSnapshotsReady = useCallback(
    (confirmed = true) => {
      if (!confirmed) return;
      setInitialLoadGeneration((current) =>
        workspaceIdentityRef.current.generation === workspaceGeneration
          ? workspaceGeneration
          : current,
      );
    },
    [workspaceGeneration],
  );

  return {
    markInitialSnapshotsReady,
    initialSnapshotsReady:
      !workspaceId || initialLoadGeneration === workspaceIdentityRef.current.generation,
  };
}
