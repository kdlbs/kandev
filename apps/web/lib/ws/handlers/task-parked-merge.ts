import {
  hasPayloadField,
  type KanbanTask,
  type TaskEventPayload,
} from "@/lib/ws/handlers/task-archive-cache";
import { resolveParkedTriple } from "@/lib/kanban/parked-projection";

/**
 * D1's (parked_epoch, revision) ordered-pair discard rule (AC-49): a strictly
 * higher epoch always wins, whatever the revision; within one epoch, discard a
 * strictly lower revision. Field-scoped — a frame that omits parked_revision
 * entirely leaves all three fields untouched rather than resetting them, and
 * task.updated always carries the three fields together
 * (addTaskParkedEventField on the backend), so there is no partial-triple case
 * to special-case here. The numeric comparison itself lives in
 * resolveParkedTriple, shared with mergeParkedProjection (session-slice.ts's
 * session-level twin) and the two REST refresh chokepoints
 * (hydrator.ts, use-all-workflow-snapshots.ts). Split into its own file to
 * keep tasks.ts under its 600-line limit, following
 * agent-session-activity-pick.ts's precedent.
 */
export function preserveParkedFields(src: KanbanTask, dst: KanbanTask, ev: TaskEventPayload): void {
  if (!hasPayloadField(ev, "parked_revision")) {
    dst.parkedOnBackgroundWork = src.parkedOnBackgroundWork;
    dst.parkedEpoch = src.parkedEpoch;
    dst.parkedRevision = src.parkedRevision;
    return;
  }

  const resolved = resolveParkedTriple(
    { parked: src.parkedOnBackgroundWork, epoch: src.parkedEpoch, revision: src.parkedRevision },
    { parked: dst.parkedOnBackgroundWork, epoch: dst.parkedEpoch, revision: dst.parkedRevision },
  );
  dst.parkedOnBackgroundWork = resolved.parked;
  dst.parkedEpoch = resolved.epoch;
  dst.parkedRevision = resolved.revision;
}
