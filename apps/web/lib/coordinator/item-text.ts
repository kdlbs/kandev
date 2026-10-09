import type { TFunction } from "i18next";
import type { AttentionTask, NeedsYouItem } from "@/lib/coordinator/attention";
import { formatAge, truncatePreview } from "@/lib/coordinator/format";

export type NeedsYouItemSeverity = "decide-now" | "review";

/**
 * Decide now for a question, permission, stall or error item; Review for a
 * proposal (docs/specs/coordinator/requirements/needs-you.md AC-002.1).
 */
export function severityFor(item: NeedsYouItem): NeedsYouItemSeverity {
  return item.kind === "proposal" ? "review" : "decide-now";
}

/**
 * The source task backing a proposal's head, when it is present in the
 * workspace's open (non-archived) tasks; otherwise the item's head is
 * "New task" with no step (AC-002.1, AC-002.9).
 */
export function resolveProposalSourceTask(
  item: NeedsYouItem,
  openTasksById: Map<string, AttentionTask>,
): AttentionTask | undefined {
  if (item.kind !== "proposal") return undefined;
  const { kind, spec } = item.proposal;
  const isCreate = kind === undefined || kind === "create_task";
  const taskId = isCreate ? spec.source_task_id : spec.task_id;
  return taskId === undefined ? undefined : openTasksById.get(taskId);
}

export type WhyClearsText = { why: string; clears: string };

/**
 * The why/clears texts by item kind (AC-COORDINATOR-NEEDS-YOU-002.2).
 */
export function whyClearsText(item: NeedsYouItem, t: TFunction): WhyClearsText {
  switch (item.kind) {
    case "proposal":
      return { why: item.proposal.spec.rationale, clears: t("coordinator:approveEditOrReject") };
    case "question":
      return { why: t("coordinator:whyQuestion"), clears: t("coordinator:clearsQuestion") };
    case "stall":
      return {
        why: t("coordinator:whyStall", { duration: formatAge(item.stall.stalled_for_ms) }),
        clears: t("coordinator:clearsStall"),
      };
    case "error": {
      const preview = item.activeError?.preview;
      const why = preview
        ? t("coordinator:whyErrorWithPreview", { preview: truncatePreview(preview) })
        : t("coordinator:whyErrorTaskFailed");
      return { why, clears: t("coordinator:clearsError") };
    }
  }
}
