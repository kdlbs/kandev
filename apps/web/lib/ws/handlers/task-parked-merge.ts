import {
  hasPayloadField,
  type KanbanTask,
  type TaskEventPayload,
} from "@/lib/ws/handlers/task-archive-cache";

/**
 * D1's (parked_epoch, revision) ordered-pair discard rule (AC-49): a strictly
 * higher epoch always wins, whatever the revision; within one epoch, discard a
 * strictly lower revision. Field-scoped — a frame that omits parked_revision
 * entirely leaves all three fields untouched rather than resetting them, and
 * task.updated always carries the three fields together
 * (addTaskParkedEventField on the backend), so there is no partial-triple case
 * to special-case here. Mirrors mergeParkedProjection, session-slice.ts's
 * session-level twin. Split into its own file to keep tasks.ts under its
 * 600-line limit, following agent-session-activity-pick.ts's precedent.
 */
export function preserveParkedFields(src: KanbanTask, dst: KanbanTask, ev: TaskEventPayload): void {
  if (!hasPayloadField(ev, "parked_revision")) {
    dst.parkedOnBackgroundWork = src.parkedOnBackgroundWork;
    dst.parkedEpoch = src.parkedEpoch;
    dst.parkedRevision = src.parkedRevision;
    return;
  }

  const existingRevision = src.parkedRevision;
  if (existingRevision === undefined) return; // nothing to compare against; keep dst as-is.

  const incomingEpoch = dst.parkedEpoch ?? 0;
  const existingEpoch = src.parkedEpoch ?? 0;
  const incomingRevision = dst.parkedRevision ?? 0;
  const incomingIsCurrent =
    incomingEpoch > existingEpoch ||
    (incomingEpoch === existingEpoch && incomingRevision >= existingRevision);
  if (incomingIsCurrent) return; // dst already holds the fresher incoming values.

  dst.parkedOnBackgroundWork = src.parkedOnBackgroundWork;
  dst.parkedEpoch = src.parkedEpoch;
  dst.parkedRevision = src.parkedRevision;
}
