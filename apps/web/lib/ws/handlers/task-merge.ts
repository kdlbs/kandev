import { mergeTaskRepositoryFields } from "@/lib/ws/handlers/task-repositories";
import { preserveOmittedDependencyFields } from "@/lib/ws/handlers/task-dependencies";
import type { KanbanTask, TaskEventPayload } from "@/lib/ws/handlers/task-archive-cache";

export function hasPayloadField(payload: TaskEventPayload, field: keyof TaskEventPayload): boolean {
  return Object.prototype.hasOwnProperty.call(payload, field);
}

function preservePrimaryExecutorFields(
  existing: KanbanTask,
  merged: KanbanTask,
  payload: TaskEventPayload,
): void {
  if (!hasPayloadField(payload, "autopilot")) merged.autopilot = existing.autopilot;
  const primarySessionCleared =
    hasPayloadField(payload, "primary_session_id") && payload.primary_session_id === null;
  if (primarySessionCleared) return;
  if (!hasPayloadField(payload, "primary_executor_id")) {
    merged.primaryExecutorId = existing.primaryExecutorId;
  }
  if (!hasPayloadField(payload, "primary_executor_profile_id")) {
    merged.primaryExecutorProfileId = existing.primaryExecutorProfileId;
  }
  if (!hasPayloadField(payload, "primary_executor_type")) {
    merged.primaryExecutorType = existing.primaryExecutorType;
  }
  if (!hasPayloadField(payload, "primary_executor_name")) {
    merged.primaryExecutorName = existing.primaryExecutorName;
  }
  if (!hasPayloadField(payload, "is_remote_executor")) {
    merged.isRemoteExecutor = existing.isRemoteExecutor;
  }
  if (!hasPayloadField(payload, "primary_agent_profile_id")) {
    merged.primaryAgentProfileId = existing.primaryAgentProfileId;
  }
  if (!hasPayloadField(payload, "primary_agent_name")) {
    merged.primaryAgentName = existing.primaryAgentName;
  }
}

// A lightweight task.updated may omit an unchanged field; only an explicit
// value (true/false, null, an object) may change the cached reading. This is
// the shared preserve guard for every field that follows that contract —
// interrupted, status_summary, and parent_id (an explicit `parent_id: null`
// means detach; see parentIDEventField in service_tasks.go).
function preserveOmittedField<K extends keyof KanbanTask>(
  existing: KanbanTask | undefined,
  merged: KanbanTask,
  payload: TaskEventPayload,
  nextTask: KanbanTask,
  field: { payloadKey: keyof TaskEventPayload; taskField: K },
): void {
  if (!hasPayloadField(payload, field.payloadKey) && nextTask[field.taskField] === undefined) {
    // The three call sites pass optional fields (interrupted, statusSummary,
    // parentTaskId), so the undefined value is the safe "not present" reading.
    merged[field.taskField] = existing?.[field.taskField] as KanbanTask[K];
  }
}

/**
 * Merge the task-level parked-on-background-work projection using the
 * (parked_epoch, parked_revision) lexicographic discard rule (spec D1,
 * mirrors mergeParkedProjection in session-slice.ts for the session-level
 * carrier). Unlike foreground_activity, the backend always serializes these
 * three fields on task.updated (no omitempty — see task-06), so there is no
 * "field omitted" case to preserve through; the only risk is an
 * out-of-order WS delivery, which the epoch/revision comparison rejects.
 */
function mergeTaskParkedFields(
  existing: KanbanTask,
  merged: KanbanTask,
  nextTask: KanbanTask,
): void {
  const incomingEpoch = nextTask.parkedEpoch;
  const existingEpoch = existing.parkedEpoch;
  const incomingRevision = nextTask.parkedRevision;
  const existingRevision = existing.parkedRevision;
  const incomingIsCurrent =
    incomingEpoch !== undefined &&
    incomingRevision !== undefined &&
    (existingEpoch === undefined ||
      existingRevision === undefined ||
      incomingEpoch > existingEpoch ||
      (incomingEpoch === existingEpoch && incomingRevision >= existingRevision));
  if (incomingIsCurrent) return;
  merged.parkedOnBackgroundWork = existing.parkedOnBackgroundWork;
  merged.parkedRevision = existingRevision;
  merged.parkedEpoch = existingEpoch;
}

export function mergeTaskUpdate(
  existing: KanbanTask | undefined,
  nextTask: KanbanTask,
  payload: TaskEventPayload,
): KanbanTask {
  if (!existing) return nextTask;
  const merged = {
    ...nextTask,
    ...mergeTaskRepositoryFields(existing, nextTask),
  };
  preserveOmittedField(existing, merged, payload, nextTask, {
    payloadKey: "parent_id",
    taskField: "parentTaskId",
  });
  // Same contract for the human assignee: a lightweight update that does not
  // mention it must not read as "unassigned". Without this, taking a task over
  // showed the new owner until the next unrelated event, then blanked.
  preserveOmittedField(existing, merged, payload, nextTask, {
    payloadKey: "assignee_user_id",
    taskField: "assigneeUserId",
  });
  preserveOmittedField(existing, merged, payload, nextTask, {
    payloadKey: "primary_session_id",
    taskField: "primarySessionId",
  });
  preserveOmittedField(existing, merged, payload, nextTask, {
    payloadKey: "primary_session_state",
    taskField: "primarySessionState",
  });
  preserveOmittedField(existing, merged, payload, nextTask, {
    payloadKey: "primary_session_pending_action",
    taskField: "primarySessionPendingAction",
  });
  preservePrimaryExecutorFields(existing, merged, payload);
  if (!hasPayloadField(payload, "metadata")) merged.metadata = existing.metadata;
  if (!hasPayloadField(payload, "labels")) merged.labels = existing.labels;
  if (!hasPayloadField(payload, "origin")) merged.origin = existing.origin;
  preserveOmittedField(existing, merged, payload, nextTask, {
    payloadKey: "task_pending_action",
    taskField: "taskPendingAction",
  });
  // Preserve the task-level activity aggregate only when the event omits it
  // entirely (e.g. a lightweight kanban.update). A task.updated that carries an
  // explicit null clears a stale background-running reading, so it must win.
  preserveOmittedField(existing, merged, payload, nextTask, {
    payloadKey: "foreground_activity",
    taskField: "foregroundActivity",
  });
  mergeTaskParkedFields(existing, merged, nextTask);
  preserveOmittedField(existing, merged, payload, nextTask, {
    payloadKey: "interrupted",
    taskField: "interrupted",
  });
  preserveOmittedField(existing, merged, payload, nextTask, {
    payloadKey: "auto_start_failed",
    taskField: "autoStartFailed",
  });
  preserveOmittedField(existing, merged, payload, nextTask, {
    payloadKey: "active_subagent_count",
    taskField: "activeSubagentCount",
  });
  preserveOmittedField(existing, merged, payload, nextTask, {
    payloadKey: "status_summary",
    taskField: "statusSummary",
  });
  preserveOmittedDependencyFields(existing, merged, payload);
  return merged;
}
