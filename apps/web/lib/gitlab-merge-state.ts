type DraftSource = { draft?: boolean; detailed_merge_status?: string };

const DRAFT_MERGE_STATUS = "draft_status";
const TRANSIENT_MERGE_STATUSES = new Set(["checking", "unchecked", "preparing"]);

/** A draft is the `draft` flag or GitLab's `draft_status` merge status; it beats every other status. */
export function isMRDraft(mr: DraftSource): boolean {
  return mr.draft === true || mr.detailed_merge_status === DRAFT_MERGE_STATUS;
}

/** True while GitLab is still computing the merge status. */
export function isMergeStatusTransient(status: string | undefined): boolean {
  return status !== undefined && TRANSIENT_MERGE_STATUSES.has(status);
}
