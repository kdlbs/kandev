import type { TaskSession } from "@/lib/types/http";
import { resolveParkedTriple } from "@/lib/kanban/parked-projection";

/**
 * Merge the runtime parked-on-background-work projection using D1's
 * (parked_epoch, revision) lexicographic discard rule (spec: Data model →
 * Revision epoch; AC-39): a strictly higher epoch always wins, whatever the
 * revision; within one epoch, discard a strictly lower revision. Field-scoped
 * (not frame-scoped) — a frame that omits parked_revision entirely leaves all
 * three fields untouched rather than resetting them, mirroring
 * mergeCancellationProjection in session-slice.ts (the precedent D1's own text
 * cites). The numeric comparison itself lives in resolveParkedTriple, shared
 * with preserveParkedFields (tasks.ts's task-level twin) and the two REST
 * refresh chokepoints (hydrator.ts, use-all-workflow-snapshots.ts). Split
 * into its own file to keep session-slice.ts under its 600-line limit,
 * following agent-session-activity-pick.ts's precedent.
 */
export function mergeParkedProjection(
  existing: TaskSession,
  incoming: TaskSession,
): Pick<TaskSession, "parked_on_background_work" | "parked_epoch" | "parked_revision"> {
  if (incoming.parked_revision === undefined) {
    return {
      parked_on_background_work: existing.parked_on_background_work,
      parked_epoch: existing.parked_epoch,
      parked_revision: existing.parked_revision,
    };
  }

  const resolved = resolveParkedTriple(
    {
      parked: existing.parked_on_background_work,
      epoch: existing.parked_epoch,
      revision: existing.parked_revision,
    },
    {
      parked: incoming.parked_on_background_work ?? existing.parked_on_background_work,
      epoch: incoming.parked_epoch,
      revision: incoming.parked_revision,
    },
  );

  return {
    parked_on_background_work: resolved.parked,
    parked_epoch: resolved.epoch,
    parked_revision: resolved.revision,
  };
}
