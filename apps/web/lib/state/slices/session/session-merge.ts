import type { TaskSession } from "@/lib/types/http";
import { mergePendingActionProjection } from "./task-session-projection-actions";

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
  return {
    ...existing,
    ...incoming,
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
