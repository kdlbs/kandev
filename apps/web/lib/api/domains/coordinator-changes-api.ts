import { fetchJson, type ApiRequestOptions } from "@/lib/api/client";
import { coordinatorPath, mutate, type Coordinator } from "@/lib/api/domains/coordinator-api";

export type ImprovementEvidence = { run_id?: string; task_id?: string };
export type ImprovementSpec = {
  title: string;
  rationale: string;
  context_before: string;
  context_after: string;
  evidence: ImprovementEvidence[];
};

// Mirrors internal/coordinator/store_pending_changes.go's PendingChange.
export type PendingChangeStatus = "pending" | "applied" | "discarded";
export type PendingChange = {
  id: string;
  coordinator_id: string;
  proposal_id: string;
  proposal_title: string;
  field: string;
  base_value: string;
  new_value: string;
  status: PendingChangeStatus;
  decided_by: string | null;
  created_at: string;
  updated_at: string;
};

export function listPendingChanges(
  workspaceId: string,
  coordinatorId: string,
  options?: ApiRequestOptions,
): Promise<{ changes: PendingChange[] }> {
  return fetchJson<{ changes: PendingChange[] }>(
    coordinatorPath(workspaceId, coordinatorId, "/pending-changes"),
    options,
  );
}

// applyPendingChange resolves to the updated coordinator on 200. A 409 body
// carries { error: "conflict", reason?: "context_changed", change }.
export function applyPendingChange(
  workspaceId: string,
  coordinatorId: string,
  changeId: string,
  options?: ApiRequestOptions,
): Promise<Coordinator> {
  return mutate<Coordinator>(
    coordinatorPath(
      workspaceId,
      coordinatorId,
      `/pending-changes/${encodeURIComponent(changeId)}/apply`,
    ),
    "POST",
    {},
    options,
  );
}

export function discardPendingChange(
  workspaceId: string,
  coordinatorId: string,
  changeId: string,
  options?: ApiRequestOptions,
): Promise<PendingChange> {
  return mutate<PendingChange>(
    coordinatorPath(
      workspaceId,
      coordinatorId,
      `/pending-changes/${encodeURIComponent(changeId)}/discard`,
    ),
    "POST",
    {},
    options,
  );
}
