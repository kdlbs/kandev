import type { QueueMeta, QueueStatus } from "./types";

type PolicyStatus = Partial<
  Pick<
    QueueStatus,
    "auto_merge_available" | "auto_merge_enabled" | "auto_merge_source" | "auto_merge_revision"
  >
>;

/** Omission is not an authoritative policy value, including on recovery reads. */
export function queuePolicyStatusMeta(status: PolicyStatus): Partial<QueueMeta> {
  const result: Partial<QueueMeta> = {};
  if (status.auto_merge_available !== undefined) {
    result.autoMergeAvailable = status.auto_merge_available;
  }
  if (status.auto_merge_enabled !== undefined) {
    result.autoMergeEnabled = status.auto_merge_enabled;
  }
  if (status.auto_merge_source !== undefined) {
    result.autoMergeSource = status.auto_merge_source;
  }
  if (status.auto_merge_revision !== undefined) {
    result.autoMergeRevision = status.auto_merge_revision;
  }
  return result;
}

function shouldPreserveQueuePolicy(previous: QueueMeta, incoming: Partial<QueueMeta>): boolean {
  const previousSource = previous.autoMergeSource;
  const nextSource = incoming.autoMergeSource;
  if (previousSource === "session" && nextSource === "global") return true;
  const previousRevision = previous.autoMergeRevision;
  const nextRevision = incoming.autoMergeRevision;
  return (
    previousSource === nextSource &&
    previousRevision !== undefined &&
    nextRevision !== undefined &&
    nextRevision < previousRevision
  );
}

/** A session-sourced policy never reverts to global, and revisions only move forward. */
export function mergeQueuePolicyMeta(
  previous: QueueMeta | undefined,
  incoming: Partial<QueueMeta>,
): Partial<QueueMeta> {
  const old =
    previous?.sessionIncarnationId === incoming.sessionIncarnationId ? previous : undefined;
  if (old && shouldPreserveQueuePolicy(old, incoming)) {
    return {
      autoMergeAvailable: old.autoMergeAvailable,
      autoMergeEnabled: old.autoMergeEnabled,
      autoMergeSource: old.autoMergeSource,
      autoMergeRevision: old.autoMergeRevision,
    };
  }
  return {
    autoMergeAvailable: incoming.autoMergeAvailable ?? old?.autoMergeAvailable,
    autoMergeEnabled: incoming.autoMergeEnabled ?? old?.autoMergeEnabled,
    autoMergeSource: incoming.autoMergeSource ?? old?.autoMergeSource,
    autoMergeRevision: incoming.autoMergeRevision ?? old?.autoMergeRevision,
  };
}
