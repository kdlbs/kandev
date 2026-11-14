import type { TFunction } from "i18next";
import type { MergePipeline, MergeTri } from "@/lib/api/domains/coordinator-merge-api";

export type ParsedChangeKey = {
  provider: string;
  host: string;
  repositoryId: string;
  number: number;
};

/** Parses `provider|host|repositoryId|number`; null for any other shape. */
export function parseChangeKey(key: string): ParsedChangeKey | null {
  const parts = key.split("|");
  if (parts.length !== 4) return null;
  const number = Number(parts[3]);
  if (!parts[0] || !Number.isInteger(number) || number <= 0) return null;
  return { provider: parts[0], host: parts[1], repositoryId: parts[2], number };
}

const SHORT_SHA_LENGTH = 7;

export function shortSha(sha: string): string {
  return sha.slice(0, SHORT_SHA_LENGTH);
}

const REASON_KEYS: Record<string, string> = {
  authority: "coordinator:mergeReasonAuthority",
  paused: "coordinator:mergeReasonPaused",
  repository_unavailable: "coordinator:mergeReasonRepositoryUnavailable",
  closed: "coordinator:mergeReasonClosed",
  merged_elsewhere: "coordinator:mergeReasonMergedElsewhere",
  stale_head: "coordinator:mergeReasonStaleHead",
  pipeline: "coordinator:mergeReasonPipeline",
  not_mergeable: "coordinator:mergeReasonNotMergeable",
  draft: "coordinator:mergeReasonDraft",
  approvals: "coordinator:mergeReasonApprovals",
  threads: "coordinator:mergeReasonThreads",
  predecessor: "coordinator:mergeReasonPredecessor",
  method: "coordinator:mergeReasonMethod",
  expired: "coordinator:mergeReasonExpired",
  provider_rejected: "coordinator:mergeReasonProviderRejected",
  not_merged: "coordinator:mergeReasonNotMerged",
  unconfirmed: "coordinator:mergeReasonUnconfirmed",
  policy_changed: "coordinator:mergeReasonPolicyChanged",
};

const CONDITION_KEYS: Record<string, string> = {
  head: "coordinator:mergeCondHead",
  pipeline: "coordinator:mergeCondPipeline",
  mergeable: "coordinator:mergeCondMergeable",
  draft: "coordinator:mergeCondDraft",
  approvals: "coordinator:mergeCondApprovals",
  threads: "coordinator:mergeCondThreads",
  predecessor: "coordinator:mergeCondPredecessor",
  method: "coordinator:mergeCondMethod",
  pause: "coordinator:mergeCondPause",
};

const UNAVAILABLE_PREFIX = "unavailable:";

/** The sentence for a refusal or waiting reason code; null for an empty code. */
export function mergeReasonText(reason: string | null | undefined, t: TFunction): string | null {
  if (!reason) return null;
  if (reason.startsWith(UNAVAILABLE_PREFIX)) {
    const conditionKey = CONDITION_KEYS[reason.slice(UNAVAILABLE_PREFIX.length)];
    const condition = conditionKey ? t(conditionKey) : t("coordinator:mergeCondUnknown");
    return t("coordinator:mergeReasonUnavailable", { condition });
  }
  const key = REASON_KEYS[reason];
  return key ? t(key) : t("coordinator:mergeReasonOther");
}

const NOTE_KEYS: Record<string, string> = {
  "Daily limit reached": "coordinator:mergeNoteDailyLimit",
  "Not contained": "coordinator:mergeNoteNotContained",
  "Not admitted": "coordinator:mergeNoteNotAdmitted",
  "Paused; a manager will decide": "coordinator:mergeNotePaused",
  "Policy changed; a manager will decide": "coordinator:mergeNotePolicyChanged",
  "Lowering automatic merge policy is pending": "coordinator:mergeNoteLowering",
};

/** The localized text of a stored manager-only note code. */
export function mergeNoteText(note: string | null | undefined, t: TFunction): string | null {
  if (!note) return null;
  return t(NOTE_KEYS[note] ?? "coordinator:mergeNoteOther");
}

const UNAVAILABLE_KEY = "coordinator:mergeValUnavailable";

const PIPELINE_KEYS: Record<MergePipeline, string> = {
  green: "coordinator:mergeValGreen",
  red: "coordinator:mergeValRed",
  pending: "coordinator:mergeValPending",
  none: "coordinator:mergeValNoChecks",
  unavailable: UNAVAILABLE_KEY,
};

export function pipelineText(value: MergePipeline, t: TFunction): string {
  return t(PIPELINE_KEYS[value] ?? PIPELINE_KEYS.unavailable);
}

type TriKeys = Record<MergeTri, string>;

const MERGEABLE_KEYS: TriKeys = {
  met: "coordinator:mergeValMergeable",
  unmet: "coordinator:mergeValNotMergeable",
  unavailable: UNAVAILABLE_KEY,
};
const CONFLICT_KEYS: TriKeys = {
  met: "coordinator:mergeValNoConflicts",
  unmet: "coordinator:mergeValConflicts",
  unavailable: UNAVAILABLE_KEY,
};
const APPROVAL_KEYS: TriKeys = {
  met: "coordinator:mergeValApproved",
  unmet: "coordinator:mergeValApprovalsNeeded",
  unavailable: UNAVAILABLE_KEY,
};
const DRAFT_KEYS: TriKeys = {
  met: "coordinator:mergeValReady",
  unmet: "coordinator:mergeValDraft",
  unavailable: UNAVAILABLE_KEY,
};

/** The proposal detail's conflict reading (present/clear/unknown) as a tri-state. */
export function conflictTri(value: "present" | "clear" | "unknown" | MergeTri): MergeTri {
  if (value === "clear") return "met";
  if (value === "present") return "unmet";
  return value === "met" || value === "unmet" ? value : "unavailable";
}

export function mergeableText(v: MergeTri, t: TFunction): string {
  return t(MERGEABLE_KEYS[v] ?? MERGEABLE_KEYS.unavailable);
}
export function conflictText(v: MergeTri, t: TFunction): string {
  return t(CONFLICT_KEYS[v] ?? CONFLICT_KEYS.unavailable);
}
export function approvalsText(v: MergeTri, t: TFunction): string {
  return t(APPROVAL_KEYS[v] ?? APPROVAL_KEYS.unavailable);
}
export function draftText(v: MergeTri, t: TFunction): string {
  return t(DRAFT_KEYS[v] ?? DRAFT_KEYS.unavailable);
}

/** A thread or behind count; null is never rendered as zero. */
export function countText(
  key: "coordinator:unresolvedThreads" | "coordinator:mergeValBehind",
  count: number | null,
  t: TFunction,
): string {
  return count === null ? t(UNAVAILABLE_KEY) : t(key, { count });
}

/** True when a rejected merge proposal's reason is a refusal code rather than a manager's text. */
export function isRefusalCode(reason: string | null | undefined): boolean {
  if (!reason) return false;
  return reason.startsWith(UNAVAILABLE_PREFIX) || reason in REASON_KEYS;
}

type MergeStatusInput = {
  status: string;
  reject_reason: string | null;
  merge?: { manager_only: boolean; note?: string; reason?: string; last_reason?: string };
};

/** The one-line status of a merge proposal. */
export function mergeStatusText(p: MergeStatusInput, t: TFunction): string {
  switch (p.status) {
    case "approving":
      return t("coordinator:mergeStatusApproving");
    case "approved":
      return t("coordinator:mergeStatusMerged");
    case "uncertain":
      return t("coordinator:mergeStatusUncertain");
    case "rejected":
      return isRefusalCode(p.reject_reason)
        ? t("coordinator:mergeStatusRefused", { reason: mergeReasonText(p.reject_reason, t) })
        : t("coordinator:mergeStatusRejected");
    case "pending":
      return p.merge?.manager_only
        ? t("coordinator:mergeStatusManagerOnly")
        : t("coordinator:mergeStatusPending");
    default:
      return t("coordinator:mergeStatusOther");
  }
}
