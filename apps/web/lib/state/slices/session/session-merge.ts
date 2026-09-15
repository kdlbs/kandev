import type { TaskSession } from "@/lib/types/http";
import { mergePendingActionProjection } from "./task-session-projection-actions";
import { getAgentGoal, isAgentGoalSnapshotNewer, mergeAgentGoalMetadata } from "@/lib/agent-goal";
import { parseTurnTimestamp } from "./turn-actions";

function asMetadataRecord(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

function hasACPAttachmentChanged(existing: TaskSession, incoming: TaskSession): boolean {
  const currentACP = asMetadataRecord(existing.metadata?.acp);
  const incomingACP = asMetadataRecord(incoming.metadata?.acp);
  const currentSessionID = typeof currentACP?.session_id === "string" ? currentACP.session_id : "";
  const incomingSessionID =
    typeof incomingACP?.session_id === "string" ? incomingACP.session_id : "";
  return (
    currentSessionID !== "" && incomingSessionID !== "" && currentSessionID !== incomingSessionID
  );
}

function goalReconciliationForAttachment(
  metadata: Record<string, unknown> | undefined,
  revision: number,
) {
  const goal = getAgentGoal(metadata);
  return {
    revision,
    cleared: goal === null,
    watermark: goal ? { createdAt: goal.createdAt, updatedAt: goal.updatedAt } : null,
  };
}

function mergeGoalReconciliation(
  existing: TaskSession,
  incoming: TaskSession,
  mergedMetadata: TaskSession["metadata"],
  attachmentChanged: boolean,
) {
  if (incoming.goal_reconciliation) return incoming.goal_reconciliation;
  if (!attachmentChanged) {
    const reconciliation = existing.goal_reconciliation;
    const incomingUpdatedAt = readGoalSnapshotUpdatedAt(incoming);
    if (
      reconciliation &&
      !reconciliation.cleared &&
      hasIncomingGoalClear(incoming) &&
      incomingUpdatedAt &&
      isAgentGoalSnapshotNewer(incomingUpdatedAt, reconciliation)
    ) {
      return {
        ...reconciliation,
        revision: reconciliation.revision + 1,
        cleared: true,
        sourceUpdatedAt: incomingUpdatedAt,
      };
    }
    return reconciliation;
  }
  const mergedACP = asMetadataRecord(mergedMetadata?.acp);
  const mergedMeta = asMetadataRecord(mergedACP?.meta);
  return goalReconciliationForAttachment(
    mergedMeta,
    (existing.goal_reconciliation?.revision ?? 0) + 1,
  );
}

function readACPUpdatedAt(value: unknown): string | undefined {
  const record = asMetadataRecord(value);
  return typeof record?.updated_at === "string" ? record.updated_at : undefined;
}

function readGoalSnapshotUpdatedAt(session: TaskSession): string | undefined {
  const candidates = [readACPUpdatedAt(session.metadata?.acp), session.updated_at];
  let newest: string | undefined;
  let newestTime: bigint | null = null;
  for (const candidate of candidates) {
    const candidateTime = parseTurnTimestamp(candidate);
    if (candidateTime === null || (newestTime !== null && candidateTime <= newestTime)) continue;
    newest = candidate;
    newestTime = candidateTime;
  }
  return newest;
}

function hasIncomingGoalClear(session: TaskSession): boolean {
  const acp = asMetadataRecord(session.metadata?.acp);
  const meta = asMetadataRecord(acp?.meta);
  return meta?.goal === null && Object.prototype.hasOwnProperty.call(meta ?? {}, "goal");
}

function mergeSessionMetadata(
  existing: TaskSession,
  incoming: TaskSession,
): TaskSession["metadata"] {
  if (incoming.metadata == null) return existing.metadata;
  const current = existing.metadata ?? {};
  const next = { ...current, ...incoming.metadata };
  const currentACP = asMetadataRecord(current.acp);
  const incomingACP = asMetadataRecord(incoming.metadata.acp);
  if (!currentACP && !incomingACP) return next;

  const currentACPRecord = currentACP ?? {};
  const incomingACPRecord = incomingACP ?? {};
  const attachmentChanged = hasACPAttachmentChanged(existing, incoming);
  const mergedACP = { ...currentACPRecord, ...incomingACPRecord };
  if (incoming.goal_reconciliation) {
    mergedACP.meta = incomingACPRecord.meta;
  } else if (incomingACPRecord.meta !== undefined || attachmentChanged) {
    const currentMeta = asMetadataRecord(currentACPRecord.meta);
    const incomingMeta = asMetadataRecord(incomingACPRecord.meta);
    mergedACP.meta = mergeAgentGoalMetadata(currentMeta, incomingMeta, {
      attachmentChanged,
      source: "hydration",
      reconciliation: existing.goal_reconciliation,
      snapshotUpdatedAt: readGoalSnapshotUpdatedAt(incoming),
    });
  } else if (currentACPRecord.meta !== undefined) {
    mergedACP.meta = currentACPRecord.meta;
  }
  next.acp = mergedACP;
  return next;
}

/** Merge the runtime cancellation projection using its process-local revision. */
function mergeCancellationProjection(
  existing: TaskSession,
  incoming: TaskSession,
): Pick<TaskSession, "cancellation_pending" | "cancellation_revision"> {
  const incomingRevision = incoming.cancellation_revision;
  const existingRevision = existing.cancellation_revision;
  const incomingIsCurrent =
    incomingRevision !== undefined &&
    (existingRevision === undefined || incomingRevision >= existingRevision);

  if (incomingIsCurrent) {
    return {
      cancellation_pending: incoming.cancellation_pending ?? existing.cancellation_pending,
      cancellation_revision: incomingRevision,
    };
  }

  if (incomingRevision === undefined && existingRevision === undefined) {
    return {
      cancellation_pending: incoming.cancellation_pending ?? existing.cancellation_pending,
      cancellation_revision: existingRevision,
    };
  }

  return {
    cancellation_pending: existing.cancellation_pending,
    cancellation_revision: existingRevision,
  };
}

/**
 * Merge the session-level parked-on-background-work projection using the
 * (parked_epoch, revision) lexicographic discard rule (spec D1): a lower
 * epoch is always stale (the process that produced it has since restarted),
 * and within one epoch a lower revision is a reordered/replayed WS delivery.
 * Mirrors mergeCancellationProjection's revision-only comparison, extended
 * with the epoch tie-break cancellation doesn't need.
 */
function mergeParkedProjection(
  existing: TaskSession,
  incoming: TaskSession,
): Pick<TaskSession, "parked_on_background_work" | "revision" | "parked_epoch"> {
  const incomingEpoch = incoming.parked_epoch;
  const existingEpoch = existing.parked_epoch;
  const incomingRevision = incoming.revision;
  const existingRevision = existing.revision;

  const incomingIsCurrent =
    incomingEpoch !== undefined &&
    incomingRevision !== undefined &&
    (existingEpoch === undefined ||
      existingRevision === undefined ||
      incomingEpoch > existingEpoch ||
      (incomingEpoch === existingEpoch && incomingRevision >= existingRevision));

  if (incomingIsCurrent) {
    return {
      parked_on_background_work:
        incoming.parked_on_background_work ?? existing.parked_on_background_work,
      revision: incomingRevision,
      parked_epoch: incomingEpoch,
    };
  }

  if (
    incomingEpoch === undefined &&
    incomingRevision === undefined &&
    existingEpoch === undefined &&
    existingRevision === undefined
  ) {
    return {
      parked_on_background_work:
        incoming.parked_on_background_work ?? existing.parked_on_background_work,
      revision: existingRevision,
      parked_epoch: existingEpoch,
    };
  }

  return {
    parked_on_background_work: existing.parked_on_background_work,
    revision: existingRevision,
    parked_epoch: existingEpoch,
  };
}

/** Merge an incoming session update with an existing session, preserving nullable fields. */
export function mergeTaskSession(existing: TaskSession, incoming: TaskSession): TaskSession {
  const cancellation = mergeCancellationProjection(existing, incoming);
  const parked = mergeParkedProjection(existing, incoming);
  const incomingRouteGeneration = incoming.route_generation;
  const existingRouteGeneration = existing.route_generation;
  const routeIsStale =
    existingRouteGeneration !== undefined &&
    (incomingRouteGeneration === undefined || incomingRouteGeneration < existingRouteGeneration);
  const pendingAction = mergePendingActionProjection(existing, incoming);
  const attachmentChanged = hasACPAttachmentChanged(existing, incoming);
  const merged = { ...existing, ...incoming };
  merged.metadata = mergeSessionMetadata(existing, incoming);
  const goalReconciliation = mergeGoalReconciliation(
    existing,
    incoming,
    merged.metadata,
    attachmentChanged,
  );
  if (goalReconciliation) merged.goal_reconciliation = goalReconciliation;
  // A backend session update never carries the frontend-only projection owner.
  // Its absence is the revocation signal for an optimistic resume rollback.
  if (incoming.resume_projection_id === undefined) delete merged.resume_projection_id;
  return {
    ...merged,
    ...cancellation,
    ...parked,
    ...(routeIsStale
      ? {
          execution_profile_id: existing.execution_profile_id,
          route_generation: existing.route_generation,
          route_state: existing.route_state,
          route_reason: existing.route_reason,
          route_error_code: existing.route_error_code,
          route_error_class: existing.route_error_class,
          route_catalogue_version: existing.route_catalogue_version,
          route_retry_ordinal: existing.route_retry_ordinal,
          route_deadline: existing.route_deadline,
          route_pending_outcome: existing.route_pending_outcome,
          downstream_acp_session_id: existing.downstream_acp_session_id,
        }
      : {}),
    ...pendingAction,
    agent_profile_snapshot: incoming.agent_profile_snapshot ?? existing.agent_profile_snapshot,
    worktree_id: incoming.worktree_id ?? existing.worktree_id,
    worktree_path: incoming.worktree_path ?? existing.worktree_path,
    worktree_branch: incoming.worktree_branch ?? existing.worktree_branch,
    workspace_path: incoming.workspace_path ?? existing.workspace_path,
    repository_id: incoming.repository_id ?? existing.repository_id,
    base_branch: incoming.base_branch ?? existing.base_branch,
    task_environment_id: incoming.task_environment_id ?? existing.task_environment_id,
  };
}
